import json
from pathlib import Path
import tempfile
import unittest

import db


class StaDatabaseTests(unittest.TestCase):
    def load(self, name, user, address, admin_secret="synthetic-secret-" * 3, max_connections=16):
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder) / 'config.yml'
            path.write_text(json.dumps({'adminSecret': admin_secret, 'mysql': {'name': name, 'user': user,
                                                  'address': address, 'password': 'fixture-only', 'maxOpenConns': max_connections}}))
            return db.load_config(path)

    def test_sta_database_and_account_only(self):
        self.assertEqual(self.load('im_sta', 'im_sta', db.RDS_ADDRESS)['mysql']['name'], 'im_sta')
        self.assertEqual(self.load('im_sta_recovery', 'im_sta_recovery', db.RDS_ADDRESS)['mysql']['name'], 'im_sta_recovery')
        for name, user, address in (
            ('im_pre', 'im_pre', db.RDS_ADDRESS),
            ('chatbot43_sta', 'chatbot43_sta', db.RDS_ADDRESS),
            ('im_sta', 'root', db.RDS_ADDRESS),
            ('im_sta', 'im_sta', 'unexpected.example.test:3306'),
        ):
            with self.subTest(name=name, user=user, address=address), self.assertRaises(ValueError):
                self.load(name, user, address)

    def test_exact_library_grants(self):
        db.validate_grants([r"GRANT SELECT ON `im\_sta`.* TO 'im_sta'@'host'"], 'im_sta')
        for scope in ('*.*', '`im_sta`.*', '`im_pre`.*'):
            with self.subTest(scope=scope), self.assertRaises(ValueError):
                db.validate_grants([f"GRANT ALL PRIVILEGES ON {scope} TO 'im_sta'@'host'"], 'im_sta')

    def test_initialization_requires_write_grant_for_exact_sta_library(self):
        readonly = [r"GRANT SELECT ON `im\_sta`.* TO 'im_sta'@'host'"]
        with self.assertRaisesRegex(ValueError, 'missing'):
            db.validate_grants(readonly, 'im_sta', initializing=True)
        db.validate_grants([r"GRANT ALL PRIVILEGES ON `im\_sta`.* TO 'im_sta'@'host'"],
                           'im_sta', initializing=True)

    def test_initialization_accepts_explicit_sta_privileges(self):
        privileges = 'SELECT, INSERT, UPDATE, DELETE, CREATE, INDEX, ALTER'
        grant = r"GRANT " + privileges + r" ON `im\_sta`.* TO 'im_sta'@'host'"
        db.validate_grants([grant], 'im_sta', initializing=True)
        with self.assertRaisesRegex(ValueError, 'DROP'):
            db.validate_grants([grant], 'im_sta', initializing=True, restoring=True)

    def test_sta_pool_limit_range(self):
        for limit in (1, 16, 24, 32):
            with self.subTest(limit=limit):
                config = self.load('im_sta', 'im_sta', db.RDS_ADDRESS, max_connections=limit)
                self.assertEqual(config['mysql']['maxOpenConns'], limit)
        for limit in (None, True, 0, 33):
            with self.subTest(limit=limit), self.assertRaisesRegex(ValueError, 'maxOpenConns'):
                self.load('im_sta', 'im_sta', db.RDS_ADDRESS, max_connections=limit)

    def test_package_schema_version(self):
        root = Path(__file__).resolve().parents[2]
        self.assertEqual(db.schema_version(root), '20261122')

    def test_sta_admin_secret_required(self):
        with self.assertRaisesRegex(ValueError, 'adminSecret'):
            self.load('im_sta', 'im_sta', db.RDS_ADDRESS, '')

    def test_schema_seeds_new_password_without_default(self):
        sql = "INSERT IGNORE INTO `accounts`(`account`,`password`)VALUES('admin','" + "a" * 40 + "');"
        updated = db.with_admin_password(sql, 'synthetic-only')
        self.assertNotIn('a' * 40, updated)
        with self.assertRaises(ValueError):
            db.with_admin_password(sql + sql, 'synthetic-only')

#!/usr/bin/env python3
"""Approved deployment operations only; never creates databases, users or grants."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile


RDS_ADDRESS = "rm-8vbq47371g7791242.mysql.zhangbei.rds.aliyuncs.com:3306"


def schema_version(root):
    sql = (root / "sql/imserver.sql").read_text(encoding="utf-8")
    versions = re.findall(r"VALUES\(\s*'jimdb_version'\s*,\s*'(\d{8})'\s*\)", sql)
    if len(versions) != 1:
        raise ValueError("expected exactly one schema version in release SQL")
    return versions[0]


def with_admin_password(sql, password):
    digest = hashlib.sha1(password.encode()).hexdigest()
    seed = r"(INSERT IGNORE INTO `accounts`\(`account`,`password`\)VALUES\('admin',')[0-9a-f]{40}('\);)"
    updated, count = re.subn(seed, lambda m: m[1] + digest + m[2], sql)
    if count != 1:
        raise ValueError("expected exactly one administrator seed in release SQL")
    return updated


def load_config(path):
    config = json.loads(Path(path).read_text(encoding="utf-8"))
    if not isinstance(config.get("adminSecret"), str) or len(config["adminSecret"].strip()) < 32:
        raise ValueError("a dedicated sta adminSecret of at least 32 characters is required")
    db = config["mysql"]
    if db["name"] not in ("im_sta", "im_sta_recovery") or db["user"] != db["name"]:
        raise ValueError("only a dedicated im_sta or im_sta_recovery account/database is allowed")
    if not re.fullmatch(r"[A-Za-z0-9.-]+:[0-9]{1,5}", db["address"]):
        raise ValueError("explicit database hostname:port required")
    if not 0 < int(db["address"].rsplit(":", 1)[1]) <= 65535:
        raise ValueError("database port out of range")
    if db["address"].lower() != RDS_ADDRESS:
        raise ValueError("database endpoint differs from the inspected sta RDS target")
    if not db["password"] or any(c in db["password"] for c in "\r\n\0"):
        raise ValueError("database password must be nonempty and contain no newline/NUL")
    return config


def option_quote(value):
    return '"' + value.replace("\\", "\\\\").replace('"', '\\"') + '"'


def validate_grants(lines, database, initializing=False):
    write_grant = False
    for line in lines:
        if "WITH GRANT OPTION" in line:
            raise ValueError("grant delegation is not permitted")
        match = re.match(r"GRANT (.*?) ON (.*?) TO ", line)
        if not match:
            raise ValueError("unreviewed role or grant format")
        privileges, scope = match.groups()
        if scope.replace("`", "") == "*.*" and privileges == "USAGE":
            continue
        # Database grant underscores are wildcards unless escaped (partial_revokes is not assumed).
        if scope.replace("`", "") != database.replace("_", "\\_") + ".*" or "GRANT OPTION" in line:
            raise ValueError("account has privileges outside the dedicated IM database")
        write_grant |= privileges == "ALL PRIVILEGES"
    if initializing and not write_grant:
        raise ValueError("initialization requires ALL PRIVILEGES on the exact sta database only")


class Client:
    def __init__(self, config):
        self.db = config["mysql"]
        host, port = self.db["address"].rsplit(":", 1)
        self.options = "[client]\nprotocol=TCP\nhost=" + host + "\nport=" + port
        self.options += "\nuser=" + self.db["user"] + "\npassword=" + option_quote(self.db["password"])
        # The MySQL CLI attempts TLS when the inspected private RDS supports it.
        self.options += "\ndefault-character-set=utf8mb4\nssl-mode=PREFERRED\nget-server-public-key=1\n"

    def run(self, sql=None, dump=False, output=None, input_file=None):
        directory = tempfile.TemporaryDirectory(prefix="im-sta-client-")
        handle, path = tempfile.mkstemp(suffix=".cnf", dir=directory.name)
        try:
            with os.fdopen(handle, "w", encoding="utf-8") as file:
                file.write(self.options)
            command = ["mysqldump" if dump else "mysql", "--defaults-file=" + path]
            environment = {key: value for key, value in os.environ.items() if not key.startswith("MYSQL_")}
            # MySQL 8.0 lacks --no-login-paths; redirect its login file into our private directory.
            environment["MYSQL_TEST_LOGIN_FILE"] = path + ".no-login"
            if dump:
                command += ["--single-transaction", "--skip-lock-tables", "--no-tablespaces", "--set-gtid-purged=OFF", "--hex-blob", "--triggers", "--skip-comments", "--column-statistics=0"]
            else:
                command += ["--batch", "--skip-column-names", "--raw", "--binary-mode", "--local-infile=0"]
            command += [self.db["name"]]
            result = subprocess.run(command, input=sql.encode() if sql is not None else None,
                                    stdin=input_file, stdout=output or subprocess.PIPE,
                                    stderr=subprocess.PIPE, timeout=120, check=False, env=environment)
            if result.returncode:
                # Never echo SQL, stderr, authentication data, or dump content into delivery logs.
                codes = re.findall(rb"ERROR (\d+)", result.stderr)
                suffix = ", MySQL " + ",".join(x.decode() for x in codes) if codes else ""
                raise RuntimeError("database client failed, exit " + str(result.returncode) + suffix)
            return result.stdout.decode() if output is None else ""
        finally:
            directory.cleanup()

    def check(self, ready=False, initializing=False):
        identity = self.run("SELECT DATABASE(),SUBSTRING_INDEX(CURRENT_USER(),'@',1);").strip()
        if identity != self.db["name"] + "\t" + self.db["user"]:
            raise ValueError("database/account identity mismatch")
        validate_grants(self.run("SHOW GRANTS;").splitlines(), self.db["name"], initializing=initializing)
        account = self.run("SHOW CREATE USER CURRENT_USER();")
        limit = re.search(r"MAX_USER_CONNECTIONS\s+(\d+)", account)
        if not limit or not 1 <= int(limit.group(1)) <= 50:
            raise ValueError("dedicated account must cap connections between 1 and 50")
        version = self.run("SELECT VERSION();").strip()
        if not version.startswith("8.0."):
            raise ValueError("only the reviewed MySQL 8.0 family is supported")
        charset = self.run("SELECT DEFAULT_CHARACTER_SET_NAME,DEFAULT_COLLATION_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME=DATABASE();").strip()
        if charset != "utf8mb4\tutf8mb4_0900_ai_ci":
            raise ValueError("IM database charset/collation mismatch")
        if ready and self.run("SELECT conf_value FROM globalconfs WHERE conf_key='jimdb_version';").strip() != schema_version(Path(__file__).resolve().parents[2]):
            raise ValueError("IM schema readiness check failed")
        return version


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("preflight", "check", "init", "backup", "restore"))
    parser.add_argument("--config", required=True, help="private JSON config mounted as config.yml")
    parser.add_argument("--file", help="private backup path; required for backup/restore")
    parser.add_argument("--admin-password-file", help="required for fresh initialization")
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[2]
    schema_version(root)
    config = load_config(args.config)
    client = Client(config)
    database = client.db["name"]
    if args.action != "restore" and database != "im_sta":
        raise ValueError("normal operations only target im_sta")
    if args.action == "restore" and database != "im_sta_recovery":
        raise ValueError("restore must use a separately approved empty recovery database")
    initializing = args.action in ("preflight", "init", "restore")
    client.check(ready=not initializing, initializing=initializing)
    if initializing:
        if client.run("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE();").strip() != "0":
            raise ValueError("refusing initialization/restore into a nonempty database")
    if args.action == "init":
        if not args.admin_password_file:
            raise ValueError("new administrator password file required")
        admin = Path(args.admin_password_file).read_text().rstrip("\n")
        if len(admin) < 24 or "\0" in admin or "\r" in admin or "\n" in admin:
            raise ValueError("fresh administrator password must be at least 24 characters")
        sql = (root / "sql/imserver.sql").read_text(encoding="utf-8")
        sql += "\n" + (Path(__file__).parent / "02-connect-address.sql").read_text()
        sql = with_admin_password(sql, admin)
        client.run(sql)
        client.check(ready=True)
    elif args.action == "backup":
        if not args.file:
            raise ValueError("private output path required")
        count = client.run("SELECT (SELECT COUNT(*) FROM information_schema.ROUTINES WHERE ROUTINE_SCHEMA=DATABASE())+(SELECT COUNT(*) FROM information_schema.EVENTS WHERE EVENT_SCHEMA=DATABASE())+(SELECT COUNT(*) FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=DATABASE());").strip()
        if count != "0":
            raise ValueError("routines/events/triggers require a reviewed backup extension")
        destination = Path(args.file)
        handle, temporary = tempfile.mkstemp(prefix=".im-backup-", dir=destination.parent)
        try:
            with os.fdopen(handle, "wb") as file:
                client.run(dump=True, output=file)
                file.flush()
                os.fsync(file.fileno())
            # Link publishes only a completed dump and atomically refuses an existing filename.
            os.link(temporary, destination)
        finally:
            os.unlink(temporary)
    elif args.action == "restore":
        if not args.file:
            raise ValueError("verified private backup path required")
        with open(args.file, "rb") as file:
            for line in file:
                if re.match(rb"\s*(USE\s|(?:CREATE|DROP|ALTER)\s+DATABASE\b|(?:CREATE|ALTER|DROP)\s+USER\b|GRANT\s)", line, re.I):
                    raise ValueError("backup contains database/account scope changes")
            file.seek(0)
            client.run(input_file=file)
        client.check(ready=True)
    print(args.action + ": checked dedicated sta database")


if __name__ == "__main__":
    try:
        main()
    except (KeyError, ValueError, RuntimeError, OSError, subprocess.SubprocessError) as error:
        raise SystemExit("blocked: " + str(error))

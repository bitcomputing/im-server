import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import redeploy


@unittest.skipUnless(os.name == "posix", "STA redeployment uses a Linux file lock")
class RedeployTests(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.root = Path(directory.name).resolve()
        self.release = self.root / "releases" / ("a" * 40)
        deploy = self.release / "deploy" / "sta"
        deploy.mkdir(parents=True)
        self.envfile = self.release / "sta-release.env"
        self.files = [deploy / "compose.yml", deploy / "compose.mindlink.yml"]
        for path in [self.root / "deployment.lock", self.envfile, *self.files, deploy / "db.py"]:
            path.touch()
        self.image = "sha256:" + "b" * 64
        self.container = {
            "Id": "old-container",
            "Image": self.image,
            "Config": {"Labels": {
                "com.docker.compose.project": "im-server-sta",
                "com.docker.compose.service": "im-server",
                "com.docker.compose.project.environment_file": str(self.envfile),
                "com.docker.compose.project.config_files": ",".join(map(str, self.files)),
            }},
            "State": {"Running": True, "Health": {"Status": "healthy"}},
        }
        capture = patch("redeploy.subprocess.check_output")
        run = patch("redeploy.subprocess.run")
        self.output = capture.start()
        self.run = run.start()
        self.addCleanup(capture.stop)
        self.addCleanup(run.stop)

    def outputs(self, image, *containers):
        config = {"name": "im-server-sta", "services": {"im-server": {"image": image}}}
        self.output.side_effect = [json.dumps([self.container]), json.dumps(config),
                                   *(json.dumps([c]) for c in containers)]

    def test_check_only_reads_active_release_and_database(self):
        self.outputs(self.image)
        with patch.dict(os.environ, {"IM_SERVER_IMAGE": "wrong", "MINDLINK_AUTHORITY_DSN": "wrong"}):
            redeploy.redeploy(self.root, check_only=True)
        rendered = self.output.call_args.args[0]
        self.assertEqual(rendered[rendered.index("--env-file") + 1], str(self.envfile))
        self.assertEqual([rendered[i + 1] for i, arg in enumerate(rendered) if arg == "-f"],
                         list(map(str, self.files)))
        self.assertNotIn("IM_SERVER_IMAGE", self.output.call_args.kwargs["env"])
        self.assertNotIn("MINDLINK_AUTHORITY_DSN", self.output.call_args.kwargs["env"])
        self.run.assert_called_once()
        self.assertIn("check", self.run.call_args.args[0])

    def test_recreates_only_im_with_the_active_image(self):
        self.outputs(self.image, dict(self.container, Id="new-container"))
        redeploy.redeploy(self.root, check_only=False)
        args = self.run.call_args.args[0]
        self.assertEqual(args[args.index("up"):], [
            "up", "-d", "--no-deps", "--no-build", "--pull", "never", "--force-recreate",
            "--wait", "--wait-timeout", "90", "im-server",
        ])
        self.assertEqual(self.run.call_count, 2)
        self.assertEqual(self.output.call_count, 3)

    def test_different_image_is_rejected_before_any_change(self):
        self.outputs("sha256:" + "c" * 64)
        with self.assertRaisesRegex(RuntimeError, "image"):
            redeploy.redeploy(self.root, check_only=False)
        self.run.assert_not_called()

    def test_foreign_project_is_rejected_before_any_change(self):
        self.container["Config"]["Labels"]["com.docker.compose.project"] = "im-server-pre"
        self.outputs(self.image)
        with self.assertRaisesRegex(RuntimeError, "project"):
            redeploy.redeploy(self.root, check_only=False)
        self.run.assert_not_called()

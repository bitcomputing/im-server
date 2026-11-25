#!/usr/bin/env python3
"""Recreate the current STA IM container with its active release and image."""

import argparse
import json
import os
from pathlib import Path
import subprocess


ROOT = Path("/opt/43chat-im-sta")
PROJECT = "im-server-sta"
SERVICE = "im-server"
CONTAINER = "im-server-sta-im-server-1"
RELEASE_ENV_KEYS = {
    "IM_SERVER_IMAGE", "IM_DB_HOST", "IM_DB_PORT",
    "MINDLINK_AUTHORITY_PROGRAM", "MINDLINK_AUTHORITY_CONFIG", "MINDLINK_AUTHORITY_DSN",
}


def inspect_container():
    output = subprocess.check_output(["docker", "inspect", "--type", "container", CONTAINER], text=True)
    container = json.loads(output)[0]
    labels = container["Config"].get("Labels") or {}
    if labels.get("com.docker.compose.project") != PROJECT or labels.get("com.docker.compose.service") != SERVICE:
        raise RuntimeError("container belongs to an unexpected Compose project or service")
    return container


def redeploy(root, check_only):
    import fcntl

    with (root / "deployment.lock").open("r+b") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        current = inspect_container()
        labels = current["Config"]["Labels"]
        envfile = Path(labels["com.docker.compose.project.environment_file"]).resolve(strict=True)
        release = envfile.parent
        if envfile.name != "sta-release.env" or release.parent != root / "releases":
            raise RuntimeError("active release environment is outside the STA releases directory")

        stack = release / "deploy" / "sta"
        base = stack / "compose.yml"
        mindlink = stack / "compose.mindlink.yml"
        files = [Path(value) for value in labels["com.docker.compose.project.config_files"].split(",")]
        if files not in ([base], [base, mindlink]) or any(path.resolve(strict=True) != path for path in files):
            raise RuntimeError("active Compose files do not belong to the expected STA release")

        args = ["docker", "compose", "-p", PROJECT, "--env-file", str(envfile)]
        for path in files:
            args.extend(["-f", str(path)])
        env = {key: value for key, value in os.environ.items()
               if key not in RELEASE_ENV_KEYS and not key.startswith("COMPOSE_")}
        config = json.loads(subprocess.check_output(args + ["config", "--format", "json"], env=env, text=True))
        if config.get("name") != PROJECT or set(config["services"]) != {SERVICE}:
            raise RuntimeError("rendered Compose project or services differ from the STA target")
        image = current["Image"]
        if not image.startswith("sha256:") or config["services"][SERVICE].get("image") != image:
            raise RuntimeError("rendered image differs from the current pinned STA image")

        subprocess.run(["python3", "-B", str(stack / "db.py"), "check",
                        "--config", str(root / "secrets" / "config.yml")], check=True)
        print(f"STA checks passed for {release.name}, image {image}", flush=True)
        if check_only:
            return

        subprocess.run(args + [
            "up", "-d", "--no-deps", "--no-build", "--pull", "never", "--force-recreate",
            "--wait", "--wait-timeout", "90", SERVICE,
        ], env=env, pass_fds=(lock.fileno(),), check=True)
        result = inspect_container()
        state = result["State"]
        if (result["Id"] == current["Id"] or result["Image"] != image or not state["Running"]
                or (state.get("Health") or {}).get("Status") != "healthy"):
            raise RuntimeError("the replacement container did not verify as healthy on the pinned image")
        print(f"STA IM is healthy in container {result['Id'][:12]}", flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="verify the active release and database")
    args = parser.parse_args()
    if os.name != "posix" or os.geteuid() != 0:
        parser.exit(1, "Run this script as root on the STA deployment host.\n")
    try:
        redeploy(ROOT, check_only=args.check)
    except BlockingIOError:
        parser.exit(1, "Another STA deployment holds the deployment lock.\n")
    except (OSError, RuntimeError, ValueError, KeyError, IndexError, subprocess.CalledProcessError) as error:
        parser.exit(1, f"STA redeploy stopped: {error}\n")


if __name__ == "__main__":
    main()

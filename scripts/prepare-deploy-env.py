#!/usr/bin/env python3
"""Prepare deployment credentials without shell/Compose password interpolation."""
import argparse
import os
from pathlib import Path
from urllib.parse import parse_qs, quote, urlsplit



def value(text):
    text = text.strip()
    if len(text) >= 2 and text[0] == text[-1] and text[0] in "\"'":
        return text[1:-1]
    return text


def prepare(source):
    settings = {}
    retained = []
    for line in source.splitlines():
        key, separator, raw = line.partition("=")
        if separator and key.strip() in ("DATABASE_URL", "DATABASE_PASSWORD"):
            settings[key.strip()] = value(raw)
        else:
            retained.append(line)
    url = settings.get("DATABASE_URL", "")
    password = settings.get("DATABASE_PASSWORD")
    # With an explicit password, discard old userinfo before URL parsing. This
    # also repairs URLs whose old password contained unescaped #, /, or ?.
    if password is not None:
        if not password:
            raise ValueError("Database password cannot be empty.")
        scheme, separator, rest = url.partition("://")
        userinfo, at, address = rest.rpartition("@")
        if not separator or not at:
            raise ValueError("DATABASE_URL must include a username and host.")
        username = userinfo.split(":", 1)[0]
        url = scheme + "://" + username + ":" + quote(password, safe="") + "@" + address
    try:
        parsed = urlsplit(url)
        port = parsed.port
    except ValueError:
        raise ValueError("Invalid DATABASE_URL host or port.") from None
    if parsed.scheme not in ("postgres", "postgresql") or not parsed.hostname or not parsed.username:
        raise ValueError("Set a PostgreSQL DATABASE_URL with username and host.")
    if parsed.fragment or any(c.isspace() for c in url) or "$" in url:
        raise ValueError("Password needs escaping. Set DATABASE_PASSWORD separately.")
    ssl = parse_qs(parsed.query).get("sslmode", [])
    if ssl not in (["require"], ["verify-ca"], ["verify-full"]):
        raise ValueError("DATABASE_URL must include sslmode=require, verify-ca, or verify-full.")
    # Passwords supplied separately are now URL-encoded, so Compose cannot
    # interpret $, quotes, backslashes, or # in them. Never print the result.
    return "\n".join(retained + ["DATABASE_URL=" + url, ""])


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("source")
    parser.add_argument("destination")
    args = parser.parse_args()
    try:
        source = Path(args.source).read_text()
        result = prepare(source)
        fd = os.open(args.destination, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
        with os.fdopen(fd, "w") as output:
            output.write(result)
    except ValueError as error:
        parser.exit(2, str(Path(args.source).resolve()) + ": " + str(error) + "\n")


if __name__ == "__main__":
    main()

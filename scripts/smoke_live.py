"""Verify the deployed app with an explicitly supplied test account; restore edits."""
import argparse
import copy
import http.cookiejar
import json
from pathlib import Path
import urllib.error
import urllib.request


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("url")
    parser.add_argument("credentials", type=Path)
    args = parser.parse_args()
    base = args.url.rstrip("/")
    credentials = json.loads(args.credentials.read_text(encoding="utf-8"))
    client = urllib.request.build_opener(
        urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar())
    )

    def request(path, body=None, expected=200):
        req = urllib.request.Request(
            base + path,
            data=None if body is None else json.dumps(body).encode(),
            headers={"Accept": "application/json", "Content-Type": "application/json", "Origin": base},
        )
        try:
            response = client.open(req, timeout=30)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            data = json.load(response)
            if response.status != expected:
                raise AssertionError(f"{path}: got {response.status}, expected {expected}: {data.get('error', '')}")
            return data

    login = {key: credentials[key] for key in ("email", "password")}
    public = request("/api/community")
    assert "profile" not in public
    request("/api/dashboard", expected=401)
    request("/matches", expected=401)
    request("/login", login)
    dashboard = request("/api/dashboard")
    profile = dashboard["profile"]
    assert profile["id"] == credentials["user_id"]
    body = {key: profile[key] for key in (
        "first_name", "last_name", "bio", "max_distance_km", "activity_visibility",
        "hours_visibility", "volunteer_with_friends", "tags", "availability"
    )}
    for row in body["availability"]:
        row["start_time"] = row["start_time"][:5]
        row["end_time"] = row["end_time"][:5]
    edited = copy.deepcopy(body)
    edited["bio"] = "Test account: deployed profile persistence verification."
    joined = "ten-hours" in profile["challenges"]
    try:
        request("/api/profile", edited)
        request("/api/challenge", {"key": "ten-hours", "join": not joined})
        request("/logout", {})
        request("/login", login)
        persisted = request("/api/dashboard")["profile"]
        assert persisted["bio"] == edited["bio"]
        assert ("ten-hours" in persisted["challenges"]) == (not joined)
        for path in ("/events", "/api/opportunities", "/api/people", "/api/review", "/api/my-events",
                     "/api/leaderboard?period=all", "/api/leaderboard?period=month&type=crews",
                     "/api/leaderboard?period=week&type=organizations"):
            assert isinstance(request(path), list), path
        request("/matches", expected=422 if profile["latitude"] is None else 200)
        request("/api/opportunities?near=true", expected=422 if profile["latitude"] is None else 200)
        request("/api/opportunities?weekday=9", expected=400)
        print("PASS: live authentication, public/private routes, profile and goal persistence across sessions, event browsing, matching validation, people, review and all ranking types.")
    finally:
        request("/api/profile", body)
        request("/api/challenge", {"key": "ten-hours", "join": joined})
        request("/logout", {})
        print("Restored the test account's profile and goal choices; no sample events or activity were created.")


if __name__ == "__main__":
    main()

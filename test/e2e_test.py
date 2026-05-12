"""
End-to-end test for bbroker.
Connects via Playwright's connect_over_cdp (same as yosemite-checker and changedetection),
creates a page, navigates to example.com, and verifies the title.
Run against a port-forwarded bbroker: kubectl port-forward svc/bbrokerd 4444:4444
"""
import asyncio
import sys
import json
import urllib.request

from playwright.async_api import async_playwright

BBROKER_URL = "ws://localhost:4444/cdtp/chrome"
STATUS_URL = "http://localhost:4444/status"


async def test_basic_navigation():
    print("=== Test: basic CDP navigation (headless) ===")
    async with async_playwright() as p:
        browser = await p.chromium.connect_over_cdp(BBROKER_URL)
        context = await browser.new_context()
        page = await context.new_page()
        await page.goto("https://example.com", timeout=30000)
        title = await page.title()
        print(f"  Title: {title!r}")
        assert "Example" in title, f"Expected 'Example' in title, got {title!r}"
        await browser.close()
    print("  PASS")


async def test_status_reflects_active_session():
    print("=== Test: /status shows active session during CDP connection ===")
    async with async_playwright() as p:
        browser = await p.chromium.connect_over_cdp(BBROKER_URL)

        # Check /status while connected
        with urllib.request.urlopen(STATUS_URL) as resp:
            data = json.loads(resp.read())
        active = data["activeSessions"]
        print(f"  Active sessions while connected: {active}")
        assert active == 1, f"Expected 1 active session, got {active}"

        await browser.close()

    # After close the session should be gone (give pod deletion a moment)
    await asyncio.sleep(3)
    with urllib.request.urlopen(STATUS_URL) as resp:
        data = json.loads(resp.read())
    active = data["activeSessions"]
    print(f"  Active sessions after close: {active}")
    assert active == 0, f"Expected 0 active sessions after close, got {active}"
    print("  PASS")


async def test_pod_created_and_deleted():
    """Verify a browser pod appears during the session and is cleaned up after."""
    import subprocess
    print("=== Test: browser pod lifecycle ===")

    async with async_playwright() as p:
        browser = await p.chromium.connect_over_cdp(BBROKER_URL)

        # Pod should exist
        result = subprocess.run(
            ["kubectl", "--context", "kind-bbroker-test",
             "-n", "bbroker-system", "get", "pods",
             "-l", "bbroker.io/component=browser",
             "--no-headers"],
            capture_output=True, text=True
        )
        pods = [l for l in result.stdout.strip().splitlines() if l]
        print(f"  Browser pods during session: {len(pods)}")
        assert len(pods) == 1, f"Expected 1 browser pod, got: {result.stdout}"
        pod_name = pods[0].split()[0]
        print(f"  Pod name: {pod_name}")

        await browser.close()

    # Pod should be gone within 15s
    for i in range(15):
        await asyncio.sleep(1)
        result = subprocess.run(
            ["kubectl", "--context", "kind-bbroker-test",
             "-n", "bbroker-system", "get", "pods",
             "-l", "bbroker.io/component=browser",
             "--no-headers"],
            capture_output=True, text=True
        )
        pods = [l for l in result.stdout.strip().splitlines() if l]
        if not pods:
            print(f"  Pod deleted after ~{i+1}s")
            break
    else:
        assert False, f"Browser pod not cleaned up after 15s: {result.stdout}"
    print("  PASS")


async def test_concurrent_sessions():
    """Open 3 concurrent sessions, verify 3 pods, close all."""
    import subprocess
    print("=== Test: 3 concurrent sessions ===")
    n = 3

    async with async_playwright() as p:
        browsers = await asyncio.gather(
            *[p.chromium.connect_over_cdp(BBROKER_URL) for _ in range(n)]
        )

        result = subprocess.run(
            ["kubectl", "--context", "kind-bbroker-test",
             "-n", "bbroker-system", "get", "pods",
             "-l", "bbroker.io/component=browser",
             "--no-headers"],
            capture_output=True, text=True
        )
        pods = [l for l in result.stdout.strip().splitlines() if l]
        print(f"  Browser pods with {n} sessions: {len(pods)}")
        assert len(pods) == n, f"Expected {n} pods, got: {result.stdout}"

        await asyncio.gather(*[b.close() for b in browsers])
    print("  PASS")


async def test_headful_pod_structure():
    """Verify ?headful=true creates a 3-container pod (browser + warden + xvfb)
    and that Chrome responds to CDP commands."""
    import subprocess
    print("=== Test: headful pod has 3 containers (browser + warden + xvfb) ===")

    async with async_playwright() as p:
        browser = await p.chromium.connect_over_cdp(BBROKER_URL + "?headful=true")

        # Check pod container count
        result = subprocess.run(
            ["kubectl", "--context", "kind-bbroker-test",
             "-n", "bbroker-system", "get", "pods",
             "-l", "bbroker.io/component=browser",
             "-o", "jsonpath={.items[0].spec.containers[*].name}"],
            capture_output=True, text=True
        )
        containers = result.stdout.strip().split()
        print(f"  Containers in headful pod: {containers}")
        assert len(containers) == 3, f"Expected 3 containers, got {len(containers)}: {containers}"
        assert "browser" in containers, "Missing 'browser' container"
        assert "warden" in containers, "Missing 'warden' container"
        assert "xvfb" in containers, "Missing 'xvfb' container"

        # Verify Chrome is actually functional
        page = await browser.new_page()
        await page.goto("https://example.com", timeout=30000)
        title = await page.title()
        print(f"  Page title in headful mode: {title!r}")
        assert "Example" in title, f"Unexpected title: {title!r}"

        await browser.close()
    print("  PASS")


async def main():
    tests = [
        test_basic_navigation,
        test_status_reflects_active_session,
        test_pod_created_and_deleted,
        test_concurrent_sessions,
        test_headful_pod_structure,
    ]
    failed = []
    for t in tests:
        try:
            await t()
        except Exception as e:
            print(f"  FAIL: {e}")
            failed.append(t.__name__)

    print()
    if failed:
        print(f"FAILED: {', '.join(failed)}")
        sys.exit(1)
    else:
        print(f"All {len(tests)} tests passed.")


if __name__ == "__main__":
    asyncio.run(main())

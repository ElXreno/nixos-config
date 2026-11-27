---
name: camoufox
description: Browse the web with Camoufox, an anti-detect Firefox, through camoufox-cli. Open pages, read them as text accessibility snapshots with element refs, click, fill, type, run JavaScript, keep cookies per session. Use whenever a page needs a real browser, JavaScript or a login, or blocks plain HTTP clients.
allowed-tools: Bash(camoufox-cli:*)
---

# Camoufox

`camoufox-cli` is `playwright-cli` driving Camoufox instead of Chrome. Every `open` starts a browser with a fresh, internally consistent device identity (OS, screen, fonts, WebGL, navigator) drawn by Camoufox and reported from C++, so pages see a normal desktop browser.

- Sessions: `camoufox-cli --session <name> <command>` (or `-s=<name>`). Parallel agents each use their own name and close only their own session; never `close-all` or `kill-all` while others work.
- `open` is headless by default; `--headed` shows the window where a display exists.
- `--os=windows|macos|linux` pins the identity's OS, `--locale=ru-RU` its language; leave both out unless a site needs them.
- Read pages with `snapshot` (text, refs like `e15`); refs go stale after navigation, take a new snapshot. Screenshots are only for a human who can see them.
- Never set a user agent, viewport or device by hand: that breaks the identity. `--browser`, `--device` and `--mobile` do not apply.
- Files a click downloads land in the session output; plain URLs are easier to fetch with `curl`.

The upstream playwright-cli guide follows, with the command renamed.


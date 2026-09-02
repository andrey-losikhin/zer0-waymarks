# Security policy

## Supported versions

Waymarks is currently pre-1.0. Security fixes are applied to the latest commit on
the `main` branch. Older commits and locally modified builds are not supported.

## Reporting a vulnerability

Please do not open a public issue for a suspected vulnerability. Use GitHub's
**Report a vulnerability** feature on the repository Security tab. If private
reporting is unavailable, contact the repository owner through the private
contact method listed on their GitHub profile.

Include the affected component and revision, reproducible steps using synthetic
data, expected and observed behavior, security impact, and a proposed mitigation
if known. Do not send real bookmark collections, browser history, cookies,
credentials, or profile archives. You should receive an acknowledgement within
seven days. A fix and disclosure timeline will be coordinated based on severity.

## Security boundaries

Waymarks does not execute bookmark values through a shell, does not directly edit
browser profile databases, and permits only HTTP(S) URLs by default. The browser
bridge is restricted by Native Messaging allowlists and validates message size
and structure. See the detailed [security model](docs/SECURITY.md).

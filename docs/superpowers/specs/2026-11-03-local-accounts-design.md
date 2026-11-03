# Local accounts — design

Date: 2026-11-03
Status: approved by the user on 2026-11-03
Sub-project 1 of "Kalzone as a self-hosted open-source platform". The others, each with
its own design later: a one-command installer, builds sent straight from the agent
(GitHub stays an option), and OAuth for the MCP server.

## 1. Why

Kalzone is becoming an open-source platform people run on their own VPS: "run it on
your VPS and get quality deploys". The person it is for, in the user's words: a
developer building projects with AI at home, with little infra experience, who buys a
VPS to put those projects in front of clients or friends and does not want to deal
with infra. Their agent does nearly everything through MCP; they barely open the
website.

Today the only way in is GitHub sign-in through a GitHub OAuth app the operator must
create first. The user named this the main problem right now: GitHub sign-in is
"strange", invasive, and pushes people away. After this sub-project a fresh instance
needs no GitHub at all to sign in, add people and give the agent a token.

The user's goal for the open-source effort is, today, a strong portfolio piece; people
outside using it for real would be the platform's success. The user's own instance
(platform.kalz.one) stays as it is and keeps working, GitHub sign-in included.

## 2. Goals, non-goals, anti-goals

Goals:

- The first owner is created from a **one-time setup link**, with a username and
  password, and leaves the setup page holding an agent token and the
  `claude mcp add …` command to paste.
- People sign in with **username and password**.
- The owner can **bring other people in** with one-time **invite links** that fix the
  team and role, and can **remove** them.
- **GitHub sign-in becomes optional**: an instance either signs people in with GitHub,
  exactly as today, or with passwords. platform.kalz.one stays in GitHub mode.
- A person who forgot their password gets back in without email.

Non-goals (this sub-project):

- The installer, and printing the setup link from it (sub-project "installer"; here the
  control plane only produces the link).
- Email of any kind (verification, magic links, reset by email). A home VPS has no
  reliable mail server.
- Passkeys and TOTP. They can come later on top of the same accounts.
- Linking a GitHub account and a password to the same user.
- Hiding teams in the UI for single-person instances; one default team is created, the
  team pages stay as they are.
- OAuth for MCP (its own sub-project).

Anti-goals, from the user — the work failed if any of these happens even when the rest
works:

- **It does not work.** A fresh instance where the setup link fails or sign-in loops.
- **It is invasive.** Anything that asks for a GitHub account or app just to get in.
- **It leaks.** A security hole that exposes data, secrets, or another person's
  projects: guessable or reusable links, user enumeration, sessions or tokens that
  survive a removal, passwords stored or logged in a readable form.

## 3. Decisions

| # | Topic | Decision | Whose |
|---|---|---|---|
| 1 | First owner | One-time setup link, then username and password. | user |
| 2 | Sign-in | Username and password; GitHub only when configured. | user |
| 2a | One mode per instance | When GitHub sign-in is configured, password accounts are off: no password form, no setup link, no invites, no reset links. The instance works exactly as today. | user |
| 3 | More people | The owner can bring someone in with an invite link; roles are the existing `member` and `maintainer`. | user |
| 4 | Agent first | Setup ends with an agent token and the `claude mcp add` command. | agreed in conversation |
| 5 | Prod approvals for the owner's first token | The setup token has bypass on by default, with a checkbox to turn it off; the deploy gate and automatic rollback are the safety net. Approvals stay for every other token as today. | agreed in conversation ("approval optional, off by default") |
| 6 | Password hashing | argon2id (`golang.org/x/crypto/argon2`), PHC string format, parameters stored with the hash. | builder's call, made here |
| 7 | Password rule | 12 to 128 characters, no composition rules (NIST 800-63B). | builder's call, made here |
| 8 | Forgotten password | An admin makes a reset link for that person; the owner who is locked out runs `controlplane setup-link --reset <login>` on the host. | builder's call, made here |
| 9 | Usernames | Same column and rules as GitHub logins (`[a-z0-9-]`, 1 to 39, lowercase). Because of 2a, local and GitHub logins never meet on one instance. | builder's call, made here |
| 10 | Invites through the agent | `createInvite`, `listInvites` and `revokeInvite` are API operations with scope `teams:admin`, and are MCP tools (an exception to "no team administration as tools"), since this person works through the agent. Removing a person stays browser-only. | builder's call, made here |

## 4. Design

### 4.1 Accounts

- `users` gains `password_hash VARCHAR(255) NULL` and `disabled_at DATETIME NULL`. A local
  user has a `password_hash` and no `github_id`; a GitHub user the reverse. `kind` stays
  `person`.
- Usernames are stored in `login`.
- **The mode** is decided at start: GitHub mode when `PLATFORM_GITHUB_CLIENT_ID` and
  `_SECRET` are both set, password mode otherwise. In GitHub mode every part of this
  design that creates or signs in a local user is off: `/login` has no password form,
  `POST /login` and `/join/<token>` answer 404, no setup link is made at start,
  `controlplane setup-link` refuses, and the invite and reset operations answer
  `409 err_github_mode` with the hint "this Kalzone signs people in with GitHub; add
  them to the team after they sign in". People join a team as today.
- Switching an instance from password mode to GitHub mode is not supported in this
  sub-project: local users keep their rows but cannot sign in while GitHub is on.
  Switching the other way leaves GitHub users unable to sign in. Both are stated in the
  configuration docs.
- Local users have no avatar: the UI shows initials where it shows GitHub avatars today.
- A disabled user cannot sign in, by password or by GitHub: the GitHub callback checks
  `disabled_at` after finding the user and refuses with the same "no access" page. Their
  sessions are deleted and their tokens revoked at the moment they are disabled (see 4.5).
- Removal is final in this sub-project: there is no re-enable, and a disabled user's
  login stays taken. Reset links, from the People page or `--reset`, are refused for
  disabled users and for GitHub users (a GitHub user never gets a password).
- `PLATFORM_ADMINS` stays a GitHub-only mechanism: it is applied in GitHub sign-in as
  today and never to local users, whose only way to admin is the setup page or an admin
  granting it.

### 4.2 Account links (setup, invite, reset)

One table, `account_links`:

| Column | Meaning |
|---|---|
| `id` | uuid |
| `kind` | `setup`, `invite`, `reset` |
| `token_hash` | `HashToken` of a 32-byte random token; the token itself is never stored |
| `team_id`, `role` | for `invite` |
| `user_id` | for `reset` |
| `created_by` | user id, empty for `setup` |
| `expires_at` | setup 24h, invite 7 days, reset 24h |
| `used_at`, `revoked_at` | a link works once and only while neither is set |

The URL is `<base>/join/<token>` for every kind. Using a link and creating or updating
the user happen in one transaction that sets `used_at` only if it is still empty, so two
browsers racing on one link create one user. An unknown, used, revoked or expired link
answers the same page: "This link is no longer valid."

### 4.3 Setup

- At start, when no user with `kind = person` exists, the control plane makes a setup
  link (revoking any earlier unused one) and logs it at `WARN` once:
  `setup: open <url> to create the owner`.
- `controlplane setup-link` (a subcommand of the same binary, run inside the container)
  prints a fresh setup link when there is no owner yet, and refuses otherwise.
- `controlplane setup-link --reset <login>` prints a reset link for that user and works
  at any time; it is the way back in for a locked-out owner, since only someone with
  shell access to the host can run it.
- The setup page asks for a username and a password (twice). Submitting it, in one
  transaction:
  - creates the user with `is_admin = true`;
  - creates the team `default` ("Default") and makes the owner its maintainer;
  - signs the owner in;
  - creates an access token named `agent` with every scope, the `default` team,
    90 days, and bypass as chosen on the page (on by default, decision 5).
- The page then shows the token once, with the ready command:
  `claude mcp add --transport http kalzone <base>/mcp --header "Authorization: Bearer <token>"`,
  and a link to the dashboard.
- Setup does not depend on GitHub settings and works with all of them empty.

### 4.4 Sign-in

- In password mode `/login` shows a username and password form; in GitHub mode it shows
  only "Continue with GitHub", as today. The demo button shows in either mode, as today.
- `POST /login` checks the password with a constant-time compare of the argon2id
  output. Wrong username, wrong password and disabled user give the same message and
  take the same time (an unknown username still runs one argon2id against a dummy hash).
- Rate limits with `internal/ratelimit`, counting failures only: 10 per IP per hour, and
  10 per (username, IP) pair per hour; past either, the form answers "Too many attempts,
  try again later" without checking the password. There is deliberately no limit per
  username alone: anyone who knows the owner's username could otherwise keep them out
  with ten requests an hour. Using a reset link clears that user's counters.
- A successful sign-in opens a session exactly like GitHub sign-in does (same cookie,
  TTL, renewal, CSRF) and honours `next` through the existing `safeNext`.
- The login page text "A team maintainer adds you to their team once you've signed in"
  becomes "Ask the owner of this Kalzone for an invite link" in password mode.

### 4.5 People

- **Invite.** A platform admin, or a maintainer of the team, creates an invite for a team
  and role on the team page or through `createInvite`. The answer holds the URL once.
  Pending invites are listed on the team page with who made them and when they expire,
  and can be revoked.
- **Accept.** `/join/<token>` for an invite asks for a username and password, creates the
  user, adds the membership, and signs them in. A person who is already signed in sees
  "Sign out to accept this invite" instead.
- **Remove.** On the People page an admin removes a person, local or GitHub: memberships deleted, sessions
  deleted, tokens revoked, `disabled_at` set, in one transaction, then the Grafana
  roster drops them on its next run. The last admin cannot be removed. History stays in
  the audit log under their login.
- **Change password.** On the person's own page, with the current password. Their other
  sessions are deleted; their tokens keep working.
- **Reset.** An admin makes a reset link for a local user from the People page. Using it
  sets a new password and deletes that user's sessions.

### 4.6 Audit

New events: `setup.completed`, `invite.created`, `invite.revoked`, `invite.accepted`,
`user.password_changed`, `user.password_reset_issued`, `user.password_reset`,
`user.removed`. None carries a password, hash or link token. Failed sign-ins are not
audit events; they are logged at `INFO` with the username and IP.

### 4.7 What stays the same

GitHub OAuth sign-in, sessions and CSRF, tokens and their scopes, approvals, the demo
guest, Grafana provisioning by login, and the GitHub App used for builds, which is
separate from sign-in and untouched here.

## 5. Testing

Integration tests on the existing suite (they sign in with fakegithub today; they gain
a password sign-in helper):

- A fresh database logs a setup link; it creates the owner, the `default` team and a
  token that works on `/api/v1/whoami` and `/mcp`; the same link then answers "no
  longer valid"; a second setup link cannot be made once an owner exists.
- Two concurrent uses of one link create one user.
- Expired, revoked and used links of every kind are refused with the same page.
- Wrong password, unknown user and disabled user give identical answers; the eleventh
  failure in an hour from one IP is refused without a password check, while the same
  username still signs in from another IP.
- A removed GitHub user is refused at the GitHub callback; reset links cannot be made
  for disabled or GitHub users; a local user whose login is in `PLATFORM_ADMINS` is not
  an admin.
- An invite creates a user in the right team and role; a member invite cannot be turned
  into a maintainer by editing the form.
- Removing a person ends their sessions and tokens at once (`/api/v1/whoami` with their
  token answers 401 right after).
- In password mode the login page has no GitHub button and `/auth/github` answers 404.
  In GitHub mode GitHub sign-in works as today, the page has no password form,
  `POST /login` and `/join/<token>` answer 404, no setup link is logged, and creating an
  invite or reset link answers `409 err_github_mode`.
- No response body, log line or audit payload contains a password, a hash, or a link
  token after its first display (asserted on the captured log and audit rows).

## 6. Left to the builder

Page layout and wording beyond what is quoted above, the argon2id parameters (sensible
2026 defaults for a small VPS), migration numbering, and how the subcommand is wired into
the existing fx composition.

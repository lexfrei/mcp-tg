# Contributing

Code conventions, architecture and the test and lint gates are described in [CLAUDE.md](CLAUDE.md). This page covers the one rule it does not: the Developer Certificate of Origin.

## Developer Certificate of Origin

Every commit in a pull request, apart from the [exemptions](#exemptions) below, must carry a sign-off certifying the following, copied verbatim from <https://developercertificate.org/>:

```text
Developer Certificate of Origin
Version 1.1

Copyright (C) 2004, 2006 The Linux Foundation and its contributors.

Everyone is permitted to copy and distribute verbatim copies of this
license document, but changing it is not allowed.


Developer's Certificate of Origin 1.1

By making a contribution to this project, I certify that:

(a) The contribution was created in whole or in part by me and I
    have the right to submit it under the open source license
    indicated in the file; or

(b) The contribution is based upon previous work that, to the best
    of my knowledge, is covered under an appropriate open source
    license and I have the right under that license to submit that
    work with modifications, whether created in whole or in part
    by me, under the same open source license (unless I am
    permitted to submit under a different license), as indicated
    in the file; or

(c) The contribution was provided directly to me by some other
    person who certified (a), (b) or (c) and I have not modified
    it.

(d) I understand and agree that this project and the contribution
    are public and that a record of the contribution (including all
    personal information I submit with it, including my sign-off) is
    maintained indefinitely and may be redistributed consistent with
    this project or the open source license(s) involved.
```

## Signing off

The sign-off is a line at the end of the commit message with your name and email:

```text
Signed-off-by: Jane Doe <jane@example.com>
```

`git commit --signoff` adds it from your `user.name` and `user.email`.

The [DCO GitHub App](https://github.com/apps/dco) checks every pull request and reports a check named `DCO`. A sign-off passes when its name matches the commit's author or committer name, and its email the author or committer email, ignoring case. When a commit fails, the check lists it with the reason, and a maintainer with write access can still mark the check as passing. To run the check again on an open pull request, submit a review with `@dcoapp recheck` on its own line.

## Fixing a branch without sign-offs

This repository does not enable the app's remediation commits, so a follow-up commit cannot sign off earlier ones and the commits themselves have to be rewritten. Add the sign-off to every commit on the branch, then update the pull request:

```bash
git fetch https://github.com/lexfrei/mcp-tg.git master
git rebase --signoff --keep-base FETCH_HEAD
git push --force-with-lease
```

The fetch names upstream explicitly because in a fork `origin` is the fork, whose `master` may lag, and rebasing onto a stale `master` would rewrite upstream commits as well. `--keep-base` leaves the branch on its current base instead of moving it onto a newer `master`. The rebase makes you the committer of each rewritten commit and signs off with the same identity, which satisfies the check even for commits authored under another email. Without `--rebase-merges` it also drops merge commits and leaves a linear branch.

## Exemptions

The app skips merge commits and commits whose author is a GitHub bot account, such as Renovate and Dependabot.

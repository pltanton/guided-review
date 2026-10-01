# <img src="docs/logo.svg" height="30" alt=""> guided-review

**Review agent-sized merge requests without skimming.**

Agents write code faster than people can read it. Merge requests got longer, reading one
properly takes an hour nobody has, and review turns into "LGTM". guided-review keeps you
as the reviewer and gives you an agent that does the tedious part.

- **A big MR becomes a dozen short steps**, grouped by behaviour and ordered the way you
  would want to read it: contract first, then the code that uses it, then its tests.
- **Every step comes explained.** Two lines on what the code does, a note on the tricky
  call, a ⚑ on the line where a bug costs money — with the reasoning one key away.
- **Say what is wrong in plain words.** "This should return an error" becomes a proper
  review comment with a severity and a ready suggestion.
- **Round two shows only what changed**, plus the author's replies to your threads with
  a suggested resolve or keep open.
- **You stay in charge.** Nothing is skipped without a reason, nothing is posted without
  your yes.

A terminal viewer in tmux next to Claude Code or Codex. GitLab and GitHub.

## Try it

Ask your agent:

> Install guided-review from https://github.com/pltanton/guided-review, follow its INSTALL.md

Then, in tmux inside the repository:

    /guided-review https://gitlab.example.com/group/project/-/merge_requests/123

Also reviews your own branch before you send it: `/guided-selfreview`.

## More

- [How it works and settings](docs/guide.md)
- [Manual installation](INSTALL.md)

Apache-2.0, see [LICENSE](LICENSE).

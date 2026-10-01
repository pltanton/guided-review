# Rules behind the checklist

What the checklist's sources say, cut down to rules you can check while reading a step.
The links are for the human; everything you need is here.

## Google engineering practices: what to look for

- **Design.** The change belongs in this codebase and in this layer. It fits the existing
  structure, and now is the right time for it.
- **Functionality.** It does what the author meant, and what they meant is good for the
  users of the code and for the people who call it. Think through edge cases and
  concurrency problems: races and deadlocks are found by reasoning, not by running it.
- **Complexity.** Lines, functions and classes are no harder than they need to be. "Too
  complex" means a reader cannot understand it quickly, or a caller is likely to
  introduce bugs when changing or calling it. Over-engineering counts: generality for a
  future that is not asked for.
- **Tests.** Unit, integration or end-to-end tests come in the same change. They would
  fail when the code breaks, they do not produce false positives, and each one asserts
  something useful. Tests are code too: no needless complexity.
- **Naming.** A name says what the thing is or does, without being so long that it is
  hard to read.
- **Comments.** Comments explain why, not what. If code needs a comment to say what it
  does, the code should be simpler instead.
- **Style and consistency.** Follow the style guide; a pure preference is a nit, not a
  blocker. Consistency with the surrounding code wins when the guide is silent.
- **Documentation.** If the change alters how people build, test, use or release the
  code, the docs change with it; deleted behaviour takes its docs with it.
- **Every line.** Look at every line you were asked to review. Read context: the whole
  file, the whole function, not just the hunk.
- **Good things.** Say what was done well, briefly.

Source: <https://google.github.io/eng-practices/review/reviewer/looking-for.html>

## SmartBear / Cisco study: pace and size

- Review at most about 200–400 lines of code at a time; defect detection drops beyond
  that.
- Read slower than about 500 lines an hour.
- Do not review for more than about 60–90 minutes in one sitting.
- Authors annotating their change before review find many defects themselves; the
  explanation in a step message plays that role here.
- A checklist catches omissions — the hardest defects to see are the code that is
  missing.

Source: <https://smartbear.com/learn/code-review/best-practices-for-peer-code-review/>

## OWASP secure code review

- **Input validation.** All data from outside the trust boundary (request, file, queue,
  env, other service) is validated on the server side: type, length, format, range,
  allow-list rather than deny-list.
- **Authentication.** Every entry point that needs it checks it; credentials and tokens
  are never logged, never in URLs, compared in constant time.
- **Authorisation.** Checked on every request on the server, per object: an id from the
  client is never enough to reach someone else's data (IDOR). Deny by default.
- **Session management.** Session ids are random, rotated on login, invalidated on
  logout and timeout.
- **Injection.** Queries are parameterised; shell commands get argument lists, never a
  string built from input; templates escape by context; paths from input are resolved
  and checked against a base directory.
- **Output encoding.** Data is encoded for the place it goes: HTML, attribute, JS, URL,
  terminal escape sequences.
- **Cryptography.** Standard libraries and algorithms only; no home-made crypto; keys
  and secrets not in code; randomness for security from a CSPRNG.
- **Error handling and logging.** Errors do not leak stack traces, queries or secrets to
  the caller. Security events are logged with enough context to investigate, and logs
  contain no passwords, tokens or personal data.
- **Data protection.** Sensitive data is encrypted in transit and at rest where required,
  and is not cached or stored longer than needed.
- **Server-side request forgery.** URLs or hosts from input are not fetched without an
  allow-list.
- **Deserialisation.** Untrusted data is not deserialised into arbitrary types.
- **Dependencies.** New dependencies are needed, maintained and free of known
  vulnerabilities.

Source: <https://cheatsheetseries.owasp.org/cheatsheets/Secure_Code_Review_Cheat_Sheet.html>

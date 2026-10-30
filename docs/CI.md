# Using Calabrass in scripts and CI

`cal run` does one task with the agent and exits. The answer goes to **stdout**, progress goes to **stderr**, and the **exit code** says how it ended, so it fits in a pipe, a shell script or a CI job.

```bash
cal run "explain what this repo does"
git diff | cal run "write a commit message for this change"
cal run "make the failing tests pass" --allow "npm test" --allow "npm run build"
cal run --json "list the TODOs" > todos.ndjson
```

## What a run is allowed to do

Nobody is there to approve anything, so a run works inside a scope you write on the command line:

- It can **read and edit files** in the folder (`-C dir`, default the current folder).
- It can run **only the commands you name with `--allow`**: `"npm test"` allows that command, `"go *"` allows any `go` command. With no `--allow`, it runs no commands at all.
- It can never reach outside the folder, install software, or act on the outside world (push, publish, send). Those need a person to approve them in the app, and an unattended run is refused them even if you list them.

Chained commands (`a && b`, pipes) are never matched by `--allow`: what you approved would not be what runs.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | It ran to the end |
| 1 | It failed (the model errored, or it ran out of steps) |
| 2 | The command line was wrong |
| 3 | **It finished what it could, but something it needed was refused.** stderr lists what; re-run with `--allow` for the commands you trust |
| 4 | `--timeout` passed first (default 15 minutes) |
| 5 | No host to talk to, could not sign in, or no model is set up |

`3` is the one worth branching on. It is not a failure of the agent; it is the agent telling you it needed permission you did not give.

## Setting up a model on a machine nobody sits at

A CI runner has no screen to add a provider on. Put it in the environment:

```bash
export CALABRASS_PROVIDER=groq          # openai, google, groq, openrouter, mistral, ...
export CALABRASS_API_KEY=...            # from your CI secrets
export CALABRASS_MODEL=llama-3.3-70b-versatile
```

`cal run` hands the key to the local host, which stores it protected; nothing is written to the job's files. The host starts by itself the first time and keeps running for the rest of the job; `cal stop` ends it.

## Output formats

- **Plain** (default): the answer on stdout; one line per action on stderr (`• run_command: npm test`, `✗ refused: ...`). `-q` silences stderr.
- **`--json`**: one JSON object per line on stdout, ending with `{"type":"result","ok":...,"exit":...,"answer":...,"blocked":[...],"steps":...,"error":...}`. stderr stays quiet.

## Piped input

Text piped in is added to the task. It is capped at 512 KB so a stray `cat bigfile` fails loudly instead of being sent to a model provider.

## Example

See [examples/calabrass-agent.yml](examples/calabrass-agent.yml): a GitHub Actions workflow that reviews each pull request's diff, and a job that tries to fix failing tests with only `npm test` pre-approved.

## Remember where the text goes

Whatever the agent reads in the folder, and anything you pipe in, may be sent to your model provider. In CI, that means your source. See [PRIVACY.md](../PRIVACY.md).

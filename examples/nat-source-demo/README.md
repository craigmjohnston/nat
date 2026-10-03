# nat-source-demo

A task-source plugin for nat that serves fixed data: three cards, grouped
Doing / Ready (with two segments, *Mine* and *Board*) / Done (lazy, two
cards once expanded). It implements every method of protocol v1 exactly
as `docs/design/task-sources/README.md` specifies them, so it is both the
manual end-to-end check for nat and gnat and a reference implementation to
read before writing a real plugin (Shortcut, say).

It is one Python 3 file using only the standard library.

## Install

Plugins are discovered at `<config dir>/plugins/<name>/nat-source-<name>`,
where the config dir is `$XDG_CONFIG_HOME/notion-agent-tracker`, else
`~/.config/notion-agent-tracker`. From the repo root:

```sh
mkdir -p ~/.config/notion-agent-tracker/plugins/demo && ln -s "$PWD/examples/nat-source-demo/nat-source-demo" ~/.config/notion-agent-tracker/plugins/demo/nat-source-demo
```

Then `nat source-list` lists it and `nat project-create --source demo`
makes a project backed by it. Remove the symlink to uninstall.

## Try it from a shell

Every call is `nat-source-demo <method>` with one JSON object on stdin. From
this directory:

```sh
P='"project":{"id":"p1","name":"Demo","working_dir":"/tmp"}'

echo "{$P}" | ./nat-source-demo describe
echo "{$P,\"expand\":[]}" | ./nat-source-demo sidebar
echo "{$P,\"expand\":[\"done\"]}" | ./nat-source-demo sidebar
echo "{$P,\"id\":\"c1\"}" | ./nat-source-demo container
echo "{$P,\"action\":\"comment\",\"target\":{\"container\":\"c1\"},\"input\":\"Looks good\"}" | ./nat-source-demo action
echo "{$P,\"action\":\"refresh\",\"target\":{}}" | ./nat-source-demo action
echo "{$P,\"action\":\"owner\",\"target\":{\"group\":\"ready/mine\"},\"input\":\"me\"}" | ./nat-source-demo action
echo "{$P,\"container\":\"c1\",\"task\":{\"id\":\"t1\",\"title\":\"Diff comments reach the agent\",\"status\":\"Todo\",\"branch\":\"\",\"pr\":\"\"},\"event\":\"created\"}" | ./nat-source-demo event
```

Pipe any of them through `python3 -m json.tool` to read the response.
The single-line form, for pasting:

```sh
echo '{"project":{"id":"p1","name":"Demo","working_dir":"/tmp"}}' | ./nat-source-demo describe
```

Errors follow the protocol: a non-zero exit and one line on stderr.

```sh
echo "{$P}" | ./nat-source-demo bogus        # exit 2: demo: unknown method 'bogus'
echo 'not json' | ./nat-source-demo describe # exit 1: demo: request is not one JSON object: …
echo "{$P,\"id\":\"zz\"}" | ./nat-source-demo container  # exit 1: demo: no card 'zz'
```

## What it does with each method

- `describe` — name `demo`, title "Demo source", tag `DM`, the
  `rectangle.on.rectangle.angled` symbol, nouns *card* / *task*, and a
  header menu with **Refresh**.
- `sidebar` — Doing (`c1`, `c2`), Ready with segments `ready/mine` and
  `ready/board` (both holding `c3`, since segments are filters over the same
  cards; each has a Rename / Owner / Remove menu), Done (`lazy`, count 36;
  `d1` and `d2` only when `expand` names `done`). Each card carries a
  coloured project badge (`NA` / `BD`), its estimate as `meta`, and a
  **Follow** menu.
- `container` — facts, a *Story* prose section, a *Comments* section with a
  composer (action `comment`), a *Links* section, and a `task_note`.
- `action` — `comment` stores the comment so the next `container` read
  shows it; `refresh` answers "Refreshed"; `rename`, `owner`, `remove` and
  `follow` answer with a message and change nothing.
- `event` — appends the event as one JSON line to the log and answers `{}`.
- `setup` — refused with `demo: nothing to set up`: the demo needs no
  credential, so its `describe` lists no setup fields.

## Where its state lives

`$XDG_STATE_HOME/nat-source-demo/`, else `~/.local/state/nat-source-demo/`:

- `events.log` — one JSON line per `event` call: time, project id,
  container, event name and the task as nat sent it. This is how to check
  nat fires events after its writes: `tail -f` it while working a task.
- `comments.json` — comments added through the composer, keyed by card id.

Delete the directory to reset it.

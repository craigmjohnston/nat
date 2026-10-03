/* gnat · Shortcut source · data */
const PROJECTS = [
  { id: "lounge", code: "LOU", name: "Lounge" },
  { id: "nat", code: "NOT", name: "notion-agent-tracker" },
  { id: "sim", code: "SIM", name: "simmer" }
];
/* Shortcut cards. group: doing · ready · done */
const CARDS = [
  { id: "4821", proj: "na", title: "Improve diff review ergonomics", group: "doing", type: "feature", est: 3, epic: "Native app parity", labels: ["diff", "agent"], owner: "Craig", requester: "Dana Wolfe", created: "18 Sep", updated: "2h ago", iteration: "Sprint 41",
    body: ["Comments left on a diff in the app never reach the working agent. They queue in the review pane until the session ends, so the agent finishes without knowing anything was said.", "Wire them through. A comment posted on any hunk lands in the agent's session as a user turn quoting the file, line range and comment body. The agent acknowledges it in the transcript, and resolving the comment in the diff marks the turn addressed.", "Acceptance: a comment posted mid-session shows up in the transcript within a second, and resolving it updates both panes."],
    comments: [["Dana Wolfe", "3d ago", "Pairs with the syntax highlighting work. Worth landing both in the same release so the diff feels finished."], ["Craig", "2d ago", "Agreed. Splitting into three tasks: comments → agent, highlighting, hunk folding."], ["Jonas Tran", "2h ago", "The agent turn format should match what nat already emits for CI logs so the transcript stays uniform."]],
    links: [["PR #418", "Diff comments reach the agent", "open"], ["PR #416", "Syntax highlighting in the diff", "open"], ["Design", "Figma · Review pane v3", "ext"]] },
  { id: "4790", proj: "bd", title: "Board mouse support", group: "doing", type: "feature", est: 2, epic: "Native app parity", labels: ["board"], owner: "Craig", requester: "Priya Deol", created: "11 Sep", updated: "Yesterday", iteration: "Sprint 41",
    body: ["The board is keyboard-only. Cards should be draggable between columns, and the hit targets need to grow so a trackpad user isn't fighting 18px rows."],
    comments: [["Priya Deol", "5d ago", "Keyboard parity is non-negotiable; mouse is additive."]],
    links: [["PR #407", "Widen board hit targets", "merged"]] },
  { id: "4802", proj: "bd", title: "Kanban column view", group: "ready", type: "feature", est: 3, epic: "Board", labels: ["board"], owner: "Craig", requester: "Priya Deol", created: "14 Sep", updated: "4d ago", iteration: "Sprint 42",
    body: ["Show the project's milestones as columns with slices as cards. Columns map to slice state. Read-only to start."], comments: [], links: [] },
  { id: "4811", proj: "bd", title: "Wheel scrolling in the Active panel", group: "ready", type: "bug", est: 1, epic: "—", labels: ["sidebar"], owner: "—", requester: "Maya Kern", created: "16 Sep", updated: "6d ago", iteration: "Sprint 42",
    body: ["Scroll wheel events over the Active list are swallowed when the list is shorter than the pane."], comments: [["Maya Kern", "6d ago", "Reproduces on 14.6 with any mouse; trackpad is fine."]], links: [] },
  { id: "4756", proj: "na", title: "Pluggable plan storage", group: "ready", type: "chore", est: 5, epic: "Platform", labels: ["storage"], owner: "—", requester: "Jonas Tran", created: "2 Sep", updated: "1w ago", iteration: "Backlog",
    body: ["Abstract the plan store so Notion and SQLite sit behind one interface. Prerequisite for any further task source."], comments: [], links: [["Doc", "RFC · plan storage seams", "ext"]] },
  { id: "4830", proj: "na", title: "Wishlist triage flow", group: "ready", type: "feature", est: 2, epic: "Native app parity", labels: ["triage"], owner: "Craig", requester: "Dana Wolfe", created: "20 Sep", updated: "2d ago", iteration: "Sprint 42",
    body: ["A place to park ideas the agent surfaces that aren't follow-ups to the current slice."], comments: [], links: [] }
];
const SC_PROJECTS = { na: ["NA", "Native App", "#4f6bd8"], bd: ["BD", "Board", "#2a9d8f"], se: ["SE", "Search", "#c2558c"] };
const DONE_CARDS = 36;
/* Shortcut sections in the sidebar: each is a saved filter over the same cards. */
const SOURCES = [
  { id: "mine", name: "My cards", filters: { owner: "me", epic: "any", label: "any", type: "any" } },
  { id: "board", name: "Board work", filters: { owner: "any", epic: "any", label: "board", type: "any" } }
];
/* Variant B: one Shortcut section; these are the filtered Ready segments. */
const SEGMENTS = [
  { id: "mine", name: "Mine", filters: { owner: "me", epic: "any", label: "any", type: "any" } },
  { id: "board", name: "Board", filters: { owner: "any", epic: "any", label: "board", type: "any" } },
  { id: "bugs", name: "Bugs", filters: { owner: "any", epic: "any", label: "any", type: "bug" } }
];
const FILTER_OPTS = { owner: ["any", "me", "unassigned"], epic: ["any", "Native app parity", "Board", "Platform"], label: ["any", "diff", "agent", "board", "sidebar", "storage", "triage"], type: ["any", "feature", "bug", "chore"] };
const matches = (c, f) => (f.owner === "any" || (f.owner === "me" ? c.owner === "Craig" : c.owner === "—")) && (f.epic === "any" || c.epic === f.epic) && (f.label === "any" || c.labels.includes(f.label)) && (f.type === "any" || c.type === f.type);
/* state: todo · working · waiting · review · pr · blocked · done. A slice belongs to a project+ms or to a card. */
const SLICES = [
  { id: "ci", project: "sim", ms: "M3: CI", msCount: "0/2", title: "Set up GitHub Actions CI", state: "working", branch: "slice/github-actions-ci", age: "12m" },
  { id: "workshop", project: "nat", ms: "M31: App parity", title: "Workshop the plan", state: "todo", brief: "" },
  { id: "parity1", project: "nat", ms: "M31: App parity", msCount: "1/3", title: "Add project-list and project-open commands", state: "todo" },
  { id: "parity2", project: "nat", ms: "M31: App parity", title: "Release a stuck slice from the app", state: "todo" },
  { id: "edits", project: "nat", ms: "M32: Review flow", msCount: "1/2", title: "Hand direct line edits back to the agent", state: "todo" },
  { id: "status", project: "nat", ms: "M33: Plan commands & Notion cleanup", msCount: "6/8", title: "Drop the status property type, and keep the select", state: "todo" },
  { id: "cycles", project: "nat", ms: "M33: Plan commands & Notion cleanup", title: "Refuse only the cycles a plan itself takes part in", state: "todo" },
  { id: "checks", project: "nat", ms: "M34: Completion checks", msCount: "0/5", title: "Add per-project completion checks to the plan", state: "todo" },
  { id: "refuse", project: "nat", ms: "M34: Completion checks", title: "Refuse a hand-back whose checks fail", state: "blocked", on: "Add per-project completion checks to the plan" },
  { id: "cifail", project: "nat", ms: "M42: CI failure feedback", msCount: "2/4", title: "Show failing checks on the board", state: "waiting", branch: "slice/show-failing-checks", age: "41m" },
  { id: "nudge", project: "nat", ms: "M42: CI failure feedback", title: "Nudge a live agent when its PR's checks fail", state: "blocked", on: "Show failing checks on the board" },
  { id: "sonarr", project: "lounge", ms: "Detail overhaul — Phase 2", msCount: "2/7", title: "Show Sonarr library truth on the TV detail page", state: "todo" },
  { id: "t-comments", card: "4821", title: "Diff comments reach the agent", state: "working", branch: "slice/diff-comments-agent", age: "23m" },
  { id: "t-syntax", card: "4821", title: "Syntax highlighting in the diff", state: "review", branch: "slice/diff-syntax", age: "1h 12m", pr: 416 },
  { id: "t-hunks", card: "4821", title: "Collapse unchanged hunks", state: "todo" },
  { id: "t-hit", card: "4790", title: "Widen board hit targets", state: "done", branch: "slice/board-hit-targets", pr: 407, age: "Yesterday" },
  { id: "t-drag", card: "4790", title: "Drag cards between columns", state: "todo" }
];
const STATE = { todo: "mute", working: "work", waiting: "hot", review: "hot", pr: "hot", blocked: "mute", done: "mute" };
const needsYou = (s) => ["waiting", "review", "pr"].includes(s.state);
const launched = (s) => !["todo", "blocked"].includes(s.state);
const handed = (s) => ["review", "pr", "done"].includes(s.state);
const hasPR = (s) => ["pr", "done"].includes(s.state);
const phaseOf = (s) => !launched(s) ? "brief" : ["working", "waiting"].includes(s.state) ? "thread" : s.state === "review" ? "changes" : "pr";
const codeOf = (s) => s.card ? "SC" : PROJECTS.find((p) => p.id === s.project).code;
const parentOf = (s) => s.card ? CARDS.find((c) => c.id === s.card) : PROJECTS.find((p) => p.id === s.project);
const cardOf = (s) => s.card ? CARDS.find((c) => c.id === s.card) : null;

const BRIEF = [
  "I want to support multiple task datasources. Currently we have the nat project datasource (SQLite/Notion) that consists of projects, milestones, and its from those that the tasks come.",
  "At work we use Shortcut (shortcut.com) to manage work. Shortcut consists of boards, teams, projects, etc. and eventually cards."
];
const TASK_BRIEF = [
  "A comment posted on any hunk in the diff pane lands in the agent's session as a user turn: file, line range, and the comment body, quoted. The agent acknowledges it in the transcript.",
  "Resolving the comment in the diff marks the turn addressed. Acceptance per the card: posted mid-session → visible in the transcript within a second."
];
const FILES = [["gnat/Review/DiffPane.swift", 212, 40, true, 1], ["gnat/Review/CommentStore.swift", 140, 12, false, 1], ["nat/internal/session/turns.go", 96, 31], ["nat/internal/session/turns_test.go", 174, 0], ["docs/design/review-pane.md", 20, 3]];
const HUNKS = [
  { file: "gnat/Review/DiffPane.swift", stat: "+212 −40", header: "@@ -88,9 +88,14 @@", rows: [
    [" ", 88, 88, "    func post(_ comment: Comment, on hunk: Hunk) {"],
    ["-", 89, "", "        store.append(comment)"],
    ["+", "", 89, "        store.append(comment)"],
    ["+", "", 90, "        session.send(.userTurn(comment.asQuote(in: hunk)))", "c"],
    ["+", "", 91, "        transcript.expectAck(for: comment.id)"],
    [" ", 90, 92, "    }"], [" ", 91, 93, ""],
    [" ", 92, 94, "    func resolve(_ id: Comment.ID) {"],
    ["+", "", 95, "        session.send(.addressed(id))"],
    [" ", 93, 96, "        store.resolve(id)"]] },
  { file: "nat/internal/session/turns.go", stat: "+96 −31", header: "@@ -41,6 +41,18 @@", rows: [
    [" ", 41, 41, "// UserTurn is anything the operator says to a live agent."],
    ["-", 42, "", "type UserTurn struct{ Body string }"],
    ["+", "", 42, "type UserTurn struct {"],
    ["+", "", 43, "\tBody  string"],
    ["+", "", 44, "\tQuote *Quote // file, range and text the turn refers to, if any"],
    ["+", "", 45, "}"],
    [" ", 43, 46, ""]] }
];
Object.assign(window, { PROJECTS, CARDS, SC_PROJECTS, DONE_CARDS, SOURCES, SEGMENTS, FILTER_OPTS, matches, SLICES, STATE, needsYou, launched, handed, hasPR, phaseOf, codeOf, parentOf, cardOf, BRIEF, TASK_BRIEF, FILES, HUNKS });

/* gnat · Shortcut source · main pane (editor · story · terminal · diff) and app root. */
function Editor({ s }) {
  const text = s.brief !== undefined ? [] : cardOf(s) ? TASK_BRIEF : BRIEF;
  return (
    <div className="editor">
      {text.map((t, i) => <p key={i}>{t}</p>)}
      {text.length ? <span className="caret"></span> : <><span className="caret"></span><span className="ph"> What should this task change?</span></>}
    </div>
  );
}
function Story({ c }) {
  return (
    <div className="scroll grow"><div className="story">
      {c.body.map((t, i) => <p key={i}>{t}</p>)}
      <div className="conv">
        <h5>Comments{c.comments.length ? ` · ${c.comments.length}` : ""}</h5>
        {c.comments.length ? c.comments.map(([by, when, t], i) => <div key={i} className="comment"><div className="by mono xs"><span className="ink">{by}</span><span className="dimtxt">{when}</span></div>{t}</div>) : <div className="dimtxt" style={{ fontSize: 13.5 }}>No comments on this card.</div>}
        <div className="composer mono xs mute">Comment on the card… <span>⌘↩</span></div>
      </div>
    </div></div>
  );
}
function Terminal({ s }) {
  const waiting = s.state === "waiting", H = handed(s), done = s.state === "done";
  return (
    <div className="term">
      <div className="grow scroll">
        <div className="t-line">● Wiring the comment through to the session as a quoted user turn.</div>
        <div className="t-line ind"><b>Edit</b>(gnat/Review/DiffPane.swift)</div>
        <div className="t-line ind mute">⎿  Allowed by auto mode classifier</div>
        <div className="t-line">● Updated <u>gnat/Review/DiffPane.swift</u> with 3 additions</div>
        <div className="t-line add ind2"><span className="ln">89</span>+ store.append(comment)</div>
        <div className="t-line add ind2"><span className="ln">90</span>+ session.send(.userTurn(comment.asQuote(in: hunk)))</div>
        <div className="t-line add ind2"><span className="ln">91</span>+ transcript.expectAck(for: comment.id)</div>
        {waiting ? <div className="t-line hot">? The CI log is 4,100 lines. Summarise failures only, or attach the full log to the slice?</div>
          : H ? <div className="t-line">✓ Handed back · {s.age} · 5 files changed · <span className="mute">slice is yours to review</span></div>
          : <div className="t-line work">* Threading… <span className="mute">({s.age} · ↓ 61.2k tokens)</span></div>}
      </div>
      {!done && <>
        <div className="t-prompt"><span className="mute">❯</span><span className="caret"></span></div>
        <div className="t-mode">⏵⏵ auto mode on <span>(shift+tab to cycle)</span><span className="grow"></span>ctx 24%</div>
      </>}
    </div>
  );
}
function Diff({ s }) {
  const review = s.state === "review";
  const [closed, setClosed] = React.useState({});
  return (
    <div className="diff scroll">
      {HUNKS.map((h, hi) => (
        <div key={h.file} className="hunk">
          <div className="f-head" onClick={() => setClosed({ ...closed, [h.file]: !closed[h.file] })}><Chev open={!closed[h.file]} /><span className="f-name">{h.file}</span><span className="xs mute">{h.stat}</span><span className="grow"></span>{review && <span className={"xs " + (hi === 0 ? "ink" : "dimtxt")}>{hi === 0 ? "✓ viewed" : "mark viewed"}</span>}</div>
          {!closed[h.file] && <><div className="h-head xs">{h.header}</div>
          {h.rows.map(([k, a, b, t, c], i) => (
            <React.Fragment key={i}>
              <div className={"d-row " + (k === "+" ? "add" : k === "-" ? "del" : "")}>
                <span className="ln">{a}</span><span className="ln">{b}</span><span className="sign">{k.trim()}</span><span className="code">{t}</span>
                {review && c && <span className="xs hot" style={{ paddingRight: 16 }}>● 1</span>}
              </div>
              {review && c && <div className="inline-c"><div className="mono xs mute">you · pending</div>Quote the hunk header too so the agent can find the line without opening the file.</div>}
            </React.Fragment>
          ))}</>}
        </div>
      ))}
      <div className="more xs">3 more files · ⌘↓ next file</div>
    </div>
  );
}

function App() {
  const [sel, setSelRaw] = React.useState(() => localStorage.getItem("gsc-sel") || "c:4821");
  const [openOv, setOpenOv] = React.useState(null);
  const [mainOv, setMainOv] = React.useState(null);
  const [dark, setDark] = React.useState(() => localStorage.getItem("gsc-dark") === "1");
  const [sources, setSources] = React.useState(SOURCES);
  const [variant, setVariant] = React.useState(() => localStorage.getItem("gsc-variant") || "a");
  const [activeCards, setActiveCards] = React.useState(() => localStorage.getItem("gsc-active") === "cards");
  React.useEffect(() => { setSources(variant === "b" ? SEGMENTS : SOURCES); }, [variant]);
  const [, bump] = React.useReducer((x) => x + 1, 0);
  const setSel = (id) => { setSelRaw(id); localStorage.setItem("gsc-sel", id); setOpenOv(null); setMainOv(null); };
  const addTask = (cid) => { const id = "t-new-" + Date.now(); SLICES.push({ id, card: cid, title: "New task", state: "todo", brief: "" }); setSel("s:" + id); };
  const isCard = sel.startsWith("c:");
  const c = isCard ? CARDS.find((x) => x.id === sel.slice(2)) || CARDS[0] : null;
  const s = !isCard ? SLICES.find((x) => x.id === sel.slice(2)) || SLICES[1] : null;
  let open, main = "none", L = false;
  if (s) {
    const p = phaseOf(s), H = handed(s); L = launched(s);
    open = openOv || { brief: p === "brief", thread: p === "thread", changes: p === "changes", pr: p === "pr" };
    main = mainOv || (p === "thread" ? "term" : H ? "diff" : "brief");
  } else open = openOv || { story: true, comments: false, links: false };
  const pickMain = (k) => { setMainOv(k); if (k === "diff" && !open.changes) setOpenOv({ ...open, changes: true }); if (k === "term" && !open.thread) setOpenOv({ ...open, thread: true }); };
  const launch = () => { s.state = "working"; s.branch = "slice/" + s.title.toLowerCase().replace(/[^a-z0-9]+/g, "-").slice(0, 30); s.age = "0m"; setOpenOv(null); setMainOv(null); bump(); };
  return (
    <>
      <div className="controls">
        <span>select</span>
        {[["c:4821", "card · doing"], ["c:4802", "card · ready"], ["s:t-hunks", "task · todo"], ["s:t-comments", "task · working"], ["s:t-syntax", "task · review"], ["s:workshop", "project slice"]].map(([id, l]) => <button key={id} className={sel === id ? "on" : ""} onClick={() => setSel(id)}>{l}</button>)}
        <span className="grow"></span>
        <span>sidebar</span>
        {[["a", "A · sections"], ["b", "B · one Shortcut"]].map(([v, l]) => <button key={v} className={variant === v ? "on" : ""} onClick={() => { setVariant(v); localStorage.setItem("gsc-variant", v); }}>{l}</button>)}
        <span>active</span>
        {[[false, "flat"], [true, "doing cards"]].map(([v, l]) => <button key={l} className={activeCards === v ? "on" : ""} onClick={() => { setActiveCards(v); localStorage.setItem("gsc-active", v ? "cards" : "flat"); }}>{l}</button>)}
        <button onClick={() => { setDark(!dark); localStorage.setItem("gsc-dark", dark ? "0" : "1"); }}>{dark ? "light" : "dark"}</button>
      </div>
      <div className={"win" + (dark ? " dark" : "")}>
        <div className="cols">
          <Sidebar sel={sel} setSel={setSel} sources={sources} setSources={setSources} addTask={addTask} variant={variant} activeCards={activeCards} />
          {s ? <Navigator s={s} open={open} setOpen={setOpenOv} main={main} setMain={setMainOv} onLaunch={launch} /> : <CardNav c={c} open={open} setOpen={setOpenOv} addTask={addTask} />}
          <main className="main">
            <div className="titlebar">
              <span className="grow"></span>
              {c && <Btn icon="ext">Open in Shortcut</Btn>}
              {s && !L && <span className="hint">⌘↩ to launch</span>}
              {s && L && <span className={"seg" + (main === "diff" ? " right" : "")}>
                <button className={main === "term" ? "on" : ""} onClick={() => pickMain("term")}>Agent</button>
                <button className={main === "diff" ? "on" : ""} onClick={() => pickMain("diff")}>Diff</button>
              </span>}
            </div>
            {c && <Story c={c} />}
            {s && main === "brief" && <Editor s={s} />}
            {s && main === "term" && <Terminal s={s} />}
            {s && main === "diff" && <Diff s={s} />}
          </main>
        </div>
        <StatusBar s={s} c={c} />
      </div>
    </>
  );
}
ReactDOM.createRoot(document.getElementById("root")).render(<App />);

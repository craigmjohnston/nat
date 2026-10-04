/* gnat · Task log navigator · static page: three states side by side. */
function LogNav({ s, main }) {
  const p = phaseOf(s), L = launched(s), H = handed(s), PR = hasPR(s), done = s.state === "done", card = cardOf(s);
  const [open, setOpen] = React.useState({ log: true, changes: false, pr: false });
  const onHead = (k) => setOpen({ ...open, [k]: !open[k] });
  return (
    <section className="nav">
      <NavTitle dot={<Dot state={s.state} />} tag={codeOf(s)} title={s.title} />
      <TaskLog s={s} open={open} onHead={onHead} onLaunch={() => {}} main={main} />
      <ChangesPR s={s} p={p} L={L} H={H} PR={PR} done={done} card={card} open={open} onHead={onHead} main={main} />
      <div className="section" style={{ flex: Object.values(open).some(Boolean) ? "0 0 0" : 1, borderBottom: 0 }}></div>
    </section>
  );
}
function Frame({ label, id, main }) {
  const s = SLICES.find((x) => x.id === id);
  return (
    <div className="frame">
      <div className="frame-l">{label}</div>
      <div className="win" data-screen-label={label}>
        <div className="cols">
          <Sidebar sel={"s:" + id} setSel={() => {}} sources={SEGMENTS} setSources={() => {}} addTask={() => {}} variant="b" activeCards={false} />
          <LogNav s={s} main={main} />
          <main className="main">
            <div className="titlebar"><span className="grow"></span>{main === "brief" ? <span className="hint">⌘↩ to launch</span> : <span className={"seg" + (main === "diff" ? " right" : "")}><button className={main === "term" ? "on" : ""}>Agent</button><button className={main === "diff" ? "on" : ""}>Diff</button></span>}</div>
            {main === "brief" && <Editor s={s} />}
            {main === "term" && <Terminal s={s} />}
            {main === "diff" && <Diff s={s} />}
          </main>
        </div>
        <StatusBar s={s} c={null} />
      </div>
    </div>
  );
}
function Page() {
  return (
    <div className="frames">
      <Frame label="Not launched" id="t-hunks" main="brief" />
      <Frame label="Working" id="t-comments" main="term" />
      <Frame label="Handed back" id="t-syntax" main="diff" />
    </div>
  );
}
ReactDOM.createRoot(document.getElementById("tl-root")).render(<Page />);

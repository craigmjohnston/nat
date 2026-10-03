/* gnat · Shortcut source · navigator: slice (Brief · Thread · Changes · PR) and card (Story · Comments · Links). */
function Section({ k, label, meta, open, selected, live = true, action, onHead, children }) {
  return (
    <div className={"section" + (open ? " open" : "")}>
      <div className={"row head sec" + (selected ? " sel" : "") + (live ? "" : " dead")} onClick={() => live && onHead(k)}>
        <Chev open={open} />
        <span>{label}</span>
        {!open && meta && <span className="meta ell">{meta}</span>}
        <span className="grow"></span>
        {action && <span className="actions">{action}</span>}
      </div>
      {open && <div className="body scroll">{children}</div>}
    </div>
  );
}
const Btn = ({ children, primary, disabled, icon, onClick }) => <button className={"btn" + (primary ? " primary" : "")} disabled={disabled} onClick={(e) => { e.stopPropagation(); onClick && onClick(); }}>{children}{icon && <I n={icon} />}</button>;
const Ev = ({ who, when, meta, tone, foot, children }) => (
  <div className="ev">
    <div className="ev-h"><span className="who">{who}</span><span className={"mono xs grow ell " + (tone || "mute")}>{meta}</span><span className="mono xs mute">{when}</span></div>
    {children && <div className="ev-b">{children}</div>}
    {foot && <div className="ev-f mono xs mute">{foot}</div>}
  </div>
);
const NavTitle = ({ dot, tag, title }) => (
  <div className="titlebar">{dot}<span className="tag">{tag}</span><span className="title">{title}</span><span className="dd"><I n="dd" size={12} /></span></div>
);

function Navigator({ s, open, setOpen, main, setMain, onLaunch }) {
  const p = phaseOf(s), L = launched(s), H = handed(s), PR = hasPR(s), done = s.state === "done", card = cardOf(s);
  const onHead = (k) => { setOpen({ ...open, [k]: !open[k] }); if (k === "thread" && L) setMain("term"); if (k === "changes" && L) setMain("diff"); };
  const threadMeta = !L ? (s.state === "blocked" ? "blocked" : "not launched") : s.state === "waiting" ? "waiting · " + s.age : s.state === "working" ? "working · " + s.age : done ? "closed" : "handed back · " + s.age;
  const brief = s.brief !== undefined ? [] : card ? TASK_BRIEF : BRIEF;
  return (
    <section className="nav">
      <NavTitle dot={<Dot state={s.state} />} tag={codeOf(s)} title={s.title} />
      <Section k="brief" label="Brief" meta="edited 3d ago" open={open.brief} onHead={onHead} action={!L && <Btn primary disabled={s.state === "blocked"} icon="arrow" onClick={onLaunch}>Launch</Btn>}>
        <div className="prose">
          {brief.length ? brief.map((t, i) => <p key={i}>{t}</p>) : <><p className="mute">Describe the changes you want to make in the editor on the right. You can list several and the agent will plan milestones and tasks for them in one go.</p><p className="mute"><span className="mono xs">⌘↩</span> launches.</p></>}
          <dl className={"facts" + (brief.length ? " sep" : "")} style={brief.length ? {} : { marginTop: 14 }}>
            {card ? <><dt>card</dt><dd>sc-{card.id} · {card.title}</dd><dt>project</dt><dd><ProjTag p={card.proj} name /></dd><dt>estimate</dt><dd>{card.est} pts</dd></> : <><dt>milestone</dt><dd>{s.ms}</dd></>}
            <dt>depends</dt><dd className={s.on ? "" : "dimtxt"}>{s.on || "none"}</dd>
            <dt>branch</dt><dd className={L ? "" : "dimtxt"}>{L ? s.branch : "assigned on launch"}</dd>
          </dl>
        </div>
      </Section>
      <Section k="thread" label="Thread" meta={threadMeta} open={open.thread} selected={main === "term" && L} onHead={onHead}>
        {!L ? (
          <div className="prose mute">
            {s.state === "blocked" ? <p>Blocked on <span className="ink">{s.on}</span>. Launch unlocks when that slice is done.</p> : <p>Launch starts Claude Code in a worktree on a new branch. Its log appears here and the terminal opens on the right.</p>}
            <div className="chips"><span className="chip">Opus 5.5 ▾</span><span className="chip">medium ▾</span></div>
          </div>
        ) : (
          <>
            <Ev who="Launched" when={done ? "Mon" : "09:12"} foot={s.branch}><span className="mono xs">Opus 5.5 · medium</span></Ev>
            {s.state === "working" && <Ev who="Agent" when={s.age} meta="working" tone="work" foot="4 files · 61k tokens · ctx 24%"><span className="mute">Editing DiffPane.swift</span></Ev>}
            {s.state === "waiting" && <Ev who="Agent" when={s.age} meta="waiting for you" tone="hot">The CI log is 4,100 lines. Summarise failures only, or attach the full log to the slice?</Ev>}
            {H && <Ev who="Agent" when={s.age} meta="handed back" foot="5 files · +642 −86 · 88k tokens">Comments now post into the session as quoted user turns and the transcript shows the acknowledgement. Resolving marks the turn addressed. 6 new tests.</Ev>}
            {PR && <Ev who="You" when={done ? "Mon" : "1h"} meta="approved" foot={`PR #${s.pr} → main`} />}
            {done && <Ev who="Merged" when="Yesterday" meta="by craig · worktree removed" />}
          </>
        )}
      </Section>
      <Section k="changes" label="Changes" live={L} open={open.changes} selected={main === "diff"} onHead={onHead}
        meta={!L ? "no branch" : p === "changes" ? "5 files · 1 viewed · 2 pending" : "5 files · +642 −86"}
        action={p === "changes" && <><Btn>Send 2 comments</Btn><Btn primary icon="check">Approve</Btn></>}>
        <div className="files">
          {FILES.map(([f, a, d, viewed, c], i) => (
            <div key={f} className={"row file" + (i === 0 ? " sel" : "")}>
              {p === "changes" && <span className={"cell check" + (viewed ? " on" : "")}>{viewed ? "✓" : ""}</span>}
              <span className="grow ell">{f}</span>
              {p === "changes" && c && <span className="xs hot">●{c}</span>}
              <span className="xs stat"><b className="add">+{a}</b> <b className="del">−{d}</b></span>
            </div>
          ))}
        </div>
      </Section>
      <Section k="pr" label="PR" live={PR} open={open.pr} onHead={onHead}
        meta={!PR ? (H ? "after approval" : "—") : done ? `#${s.pr} · merged` : `#${s.pr} · open · 0 of 2 checks`}
        action={PR && !done && <Btn disabled>Merge</Btn>}>
        <div className="prose">
          <h5>Checks</h5>
          <div className="mono xs lines">{done ? <><div><span className="ok">✓</span> gnat | Test</div><div><span className="ok">✓</span> nat | go test</div></> : <><div><span className="work">◐</span> gnat | Test <span className="mute">· running 4m</span></div><div><span className="mute">○</span> nat | go test <span className="mute">· queued</span></div></>}</div>
          {card && <><h5>Shortcut</h5><div className="mute">Linked to sc-{card.id}. Merging moves the card to Done when it's the last open task.</div></>}
          <div className="chips"><Btn icon="ext">Open in GitHub</Btn></div>
        </div>
      </Section>
      <div className="section" style={{ flex: Object.values(open).some(Boolean) ? "0 0 0" : 1, borderBottom: 0 }}></div>
    </section>
  );
}

function CardNav({ c, open, setOpen, addTask }) {
  const onHead = (k) => setOpen({ ...open, [k]: !open[k] });
  const tasks = SLICES.filter((s) => s.card === c.id), done = tasks.filter((s) => s.state === "done").length;
  return (
    <section className="nav">
      <NavTitle dot={<span className="cell" style={{ color: "var(--ink-2)", display: "flex" }}><I n="shortcut" size={13} /></span>} tag="SC" title={c.title} />
      <Section k="story" label="Story" meta={`sc-${c.id} · ${c.type} · ${c.est} pts`} open={open.story} onHead={onHead} action={<Btn primary icon="plus" onClick={() => addTask(c.id)}>New task</Btn>}>
        <div className="prose">
          <dl className="facts">
            <dt>id</dt><dd>sc-{c.id}</dd>
            <dt>project</dt><dd><ProjTag p={c.proj} name /></dd>
            <dt>state</dt><dd>{c.group === "doing" ? "In Development" : c.group === "ready" ? "Ready for Dev" : "Done"}</dd>
            <dt>type</dt><dd>{c.type}</dd>
            <dt>epic</dt><dd className={c.epic === "—" ? "dimtxt" : ""}>{c.epic}</dd>
            <dt>labels</dt><dd>{c.labels.map((l) => <span key={l} className="lbl">{l}</span>)}</dd>
            <dt>owner</dt><dd className={c.owner === "—" ? "dimtxt" : ""}>{c.owner === "—" ? "unassigned" : c.owner}</dd>
            <dt>requester</dt><dd>{c.requester}</dd>
            <dt>created</dt><dd>{c.created}</dd>
            <dt>updated</dt><dd>{c.updated}</dd>
            <dt>tasks</dt><dd className={tasks.length ? "" : "dimtxt"}>{tasks.length ? `${done}/${tasks.length} done` : "none yet"}</dd>
          </dl>
        </div>
      </Section>
      <Section k="links" label="Links" meta={c.links.length ? `${c.links.length}` : "none"} open={open.links} onHead={onHead}>
        <div style={{ padding: "6px 0" }}>
          {c.links.length ? c.links.map(([k, t, st], i) => <div key={i} className="link"><I n={st === "ext" ? "ext" : "branch"} /><span className="k">{k}</span><span className="grow ell">{t}</span>{st !== "ext" && <span className="st">{st}</span>}</div>) : <div className="link dimtxt">No linked PRs or documents.</div>}
        </div>
      </Section>
      <div className="section" style={{ flex: Object.values(open).some(Boolean) ? "0 0 0" : 1, borderBottom: 0 }}></div>
    </section>
  );
}
Object.assign(window, { Navigator, CardNav, Btn });

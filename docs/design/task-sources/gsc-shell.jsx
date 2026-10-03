/* gnat · Shortcut source · shell: icons, sidebar, status bar. */
const PATHS = {
  gear: "M7 4.6a2.4 2.4 0 1 0 0 4.8 2.4 2.4 0 0 0 0-4.8zM7 1.2v1.3M7 11.5v1.3M1.2 7h1.3M11.5 7h1.3M2.9 2.9l.9.9M10.2 10.2l.9.9M2.9 11.1l.9-.9M10.2 3.8l.9-.9",
  plus: "M7 2.5v9M2.5 7h9",
  x: "M3.5 3.5l7 7M10.5 3.5l-7 7",
  folder: "M1.5 4a1 1 0 0 1 1-1h3l1.5 1.5h5a1 1 0 0 1 1 1V11a1 1 0 0 1-1 1h-9.5a1 1 0 0 1-1-1z",
  folderNew: "M7 12H2.5a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1h3l1.5 1.5h5a1 1 0 0 1 1 1V7M11 9v4M9 11h4",
  ms: "M1.5 4a1 1 0 0 1 1-1h3l1.5 1.5h5a1 1 0 0 1 1 1V11a1 1 0 0 1-1 1h-9.5a1 1 0 0 1-1-1zM1.5 7.5h11",
  card: "M2 3.5h10a.8.8 0 0 1 .8.8v5.4a.8.8 0 0 1-.8.8H2a.8.8 0 0 1-.8-.8V4.3a.8.8 0 0 1 .8-.8zM4 6.2h6M4 8.2h3.5",
  filter: "M1.5 3h11M3.5 7h7M5.5 11h3",
  search: "M6 1.8a4.2 4.2 0 1 0 0 8.4 4.2 4.2 0 0 0 0-8.4zM9.1 9.1l3.2 3.2",
  dd: "M3.5 5.5L7 9l3.5-3.5",
  arrow: "M2 7h10M8 3l4 4-4 4",
  check: "M2 7.5l3.3 3.3L12 3.5",
  ext: "M8 2h4v4M12 2L6.5 7.5M10 8.5V11a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1h2.5",
  branch: "M4 2v10M4 2a1.5 1.5 0 1 0 0 .01M4 12a1.5 1.5 0 1 0 0 .01M10 4a1.5 1.5 0 1 0 0 .01M10 5.5c0 2.5-6 2-6 4.5",
  logo: "M2 3.5l10 7M2 10.5l10-7M3.5 3.5a1.5 1.5 0 1 0 0 .01M3.5 10.5a1.5 1.5 0 1 0 0 .01",
  dots: "M3 7a.6.6 0 1 0 0 .01M7 7a.6.6 0 1 0 0 .01M11 7a.6.6 0 1 0 0 .01"
};
const I = ({ n, size = 14, className = "", style }) => n === "shortcut"
  ? <svg className={"ic sc " + className} viewBox="0 0 48 48" style={{ width: size, height: size, ...style }}><path fillRule="evenodd" clipRule="evenodd" d="M18.2765 8.46875H39.8392L30.0769 19.183L39.652 28.7301L29.7873 39.5561L8.15918 39.5506L17.9624 28.7915L8.42517 19.2828L18.2765 8.46875ZM19.7228 30.5467L13.8141 37.0315L26.2301 37.0346L19.7228 30.5467ZM29.2139 36.498L21.3993 28.7067L28.4005 21.0229L36.2151 28.8147L29.2139 36.498ZM26.6401 19.2677L19.6388 26.9516L11.8619 19.1979L18.8627 11.5129L26.6401 19.2677ZM28.3166 17.4277L34.183 10.9893H21.8593L28.3166 17.4277Z" /></svg>
  : <svg className={"ic " + className} viewBox="0 0 14 14" style={{ width: size, height: size, ...style }}><path d={PATHS[n]} /></svg>;
const ProjTag = ({ p, name }) => { const [code, full, col] = SC_PROJECTS[p]; return <span className="ptag" title={full} style={{ color: col, background: `color-mix(in srgb, ${col} 14%, transparent)` }}>{name ? full : code}</span>; };
const Chev = ({ open }) => <span className={"cell chev" + (open ? " open" : "")}><svg viewBox="0 0 10 10"><path d="M3.5 2l3 3-3 3" /></svg></span>;
const Dot = ({ state }) => {
  if (state === "todo") return <span className="dot hollow"></span>;
  if (state === "blocked") return <span className="dot ring"></span>;
  if (state === "done") return <span className="dot dim"></span>;
  return <span className={"dot " + STATE[state] + (state === "working" ? " pulse" : "")}></span>;
};

/* Fixed-position popover portaled into the window so it escapes the sidebar's overflow clip. */
function Pop({ anchor, close, width = 208, children }) {
  const ref = React.useRef();
  React.useEffect(() => { const h = (e) => { if (ref.current && !ref.current.contains(e.target) && !(anchor && anchor.contains(e.target))) close(); }; document.addEventListener("mousedown", h); return () => document.removeEventListener("mousedown", h); }, []);
  const r = anchor ? anchor.getBoundingClientRect() : { bottom: 0, right: 0 };
  return ReactDOM.createPortal(<div className="pop" ref={ref} style={{ top: Math.min(r.bottom + 6, window.innerHeight - 160), left: r.right + 4 - width, width }} onClick={(e) => e.stopPropagation()}>{children}</div>, document.querySelector(".win") || document.body);
}
function Menu({ anchor, close, items }) {
  return <Pop anchor={anchor} close={close} width={190}>{items.map(([l, f, danger]) => <div key={l} className={"pop-row menu" + (danger ? " danger" : "")} onClick={() => { close(); f && f(); }}>{l}</div>)}</Pop>;
}
function FilterPop({ f, setF, close, anchor }) {
  const cycle = (k) => { const o = FILTER_OPTS[k]; setF({ ...f, [k]: o[(o.indexOf(f[k]) + 1) % o.length] }); };
  return (
    <Pop anchor={anchor} close={close}>
      {Object.keys(FILTER_OPTS).map((k) => (
        <div key={k} className="pop-row" onClick={() => cycle(k)}><span className="k">{k}</span><span className={"v" + (f[k] === "any" ? " any" : "")}>{f[k]}</span><I n="dd" size={10} /></div>
      ))}
      <div className="pop-foot">click a value to change it</div>
    </Pop>
  );
}

function SourceHead({ src, open, onTog, onRename, onFilters, onDuplicate, onRemove, seg }) {
  const [edit, setEdit] = React.useState(false);
  const [pop, setPop] = React.useState(null);
  const fbtn = React.useRef(), mbtn = React.useRef();
  return (
    <div className={seg ? "row grp segh" : "row head src"} style={seg ? { paddingLeft: 20 } : {}} onClick={onTog} onDoubleClick={(e) => { e.stopPropagation(); setEdit(true); }}>
      <Chev open={open} />
      {!seg && <span className="cell" style={{ color: "var(--ink-2)" }}><I n="shortcut" /></span>}
      {edit ? <input className={"rename" + (seg ? " sm" : "")} autoFocus defaultValue={src.name} onClick={(e) => e.stopPropagation()} onBlur={(e) => { onRename(e.target.value || src.name); setEdit(false); }} onKeyDown={(e) => { if (e.key === "Enter") e.target.blur(); if (e.key === "Escape") setEdit(false); }} /> : <span className="grow clip">{src.name}</span>}
      <span ref={fbtn} className="plus" style={{ display: seg ? undefined : "flex" }} title="Filters" onClick={(e) => { e.stopPropagation(); setPop(pop === "f" ? null : "f"); }}><I n="filter" size={seg ? 12 : 14} /></span>
      <span ref={mbtn} className="plus" style={{ display: seg ? undefined : "flex" }} title="Section" onClick={(e) => { e.stopPropagation(); setPop(pop === "m" ? null : "m"); }}><I n="dots" size={seg ? 12 : 14} /></span>
      {pop === "f" && <FilterPop f={src.filters} setF={onFilters} close={() => setPop(null)} anchor={fbtn.current} />}
      {pop === "m" && <Menu anchor={mbtn.current} close={() => setPop(null)} items={[["Rename", () => setEdit(true)], [seg ? "Duplicate segment" : "Duplicate section", onDuplicate], [seg ? "Remove segment" : "Remove section", onRemove, true]]} />}
    </div>
  );
}

function Sidebar({ sel, setSel, sources, setSources, addTask, variant, activeCards }) {
  const [fold, setFold] = React.useState({ scratch: true, "g:done": true, "p:lounge": true, "p:sim": true });
  const scrollRef = React.useRef();
  React.useEffect(() => { const el = scrollRef.current, r = el && el.querySelector(".row.sel"); if (!r) return; const t = r.offsetTop; if (t < el.scrollTop || t + 26 > el.scrollTop + el.clientHeight) el.scrollTop = Math.max(0, t - el.clientHeight / 2); }, [sel]);
  const tog = (k) => setFold({ ...fold, [k]: !fold[k] });
  const active = SLICES.filter((s) => ["working", "waiting", "review", "pr"].includes(s.state) && !(activeCards && s.card)).sort((a, b) => (needsYou(b) ? 1 : 0) - (needsYou(a) ? 1 : 0));
  const doingCards = CARDS.filter((c) => c.group === "doing");
  const needs = SLICES.filter(needsYou).length;
  const Row = ({ s, depth, tag }) => (
    <div className={"row" + (sel === "s:" + s.id ? " sel" : "") + (s.state === "done" || s.state === "blocked" ? " dim" : "")} style={{ paddingLeft: 12 + depth * 8 }} onClick={() => setSel("s:" + s.id)}>
      <span className="cell"><Dot state={s.state} /></span>
      {tag && <span className="tag">{tag}</span>}
      <span className="grow clip">{s.title}</span>
      {tag && <span className="x" onClick={(e) => e.stopPropagation()}><I n="x" size={12} /></span>}
    </div>
  );
  const Head = ({ k, label, count, right }) => (
    <div className="row head" onClick={() => tog(k)}>
      <Chev open={!fold[k]} />
      <span>{label}</span>
      {count ? <span className="cnt work">{count}</span> : null}
      <span className="grow"></span>
      {right}
    </div>
  );
  const stop = (f) => (e) => { e.stopPropagation(); f && f(); };
  const groups = [["doing", "Doing"], ["ready", "Ready"]];
  const upd = (id, patch) => setSources(sources.map((x) => x.id === id ? { ...x, ...patch } : x));
  const addSource = (from) => setSources([...sources, { id: "src" + Date.now(), name: from ? from.name + " copy" : (variant === "b" ? "Ready" : "Shortcut"), filters: from ? { ...from.filters } : { owner: "any", epic: "any", label: "any", type: "any" } }]);
  const [tbMenu, setTbMenu] = React.useState(false);
  const tbPlus = React.useRef();
  const [scMenu, setScMenu] = React.useState(false);
  const scDots = React.useRef();
  const CardRows = ({ list, empty }) => <>
    {list.map((c) => {
      const tasks = SLICES.filter((s) => s.card === c.id);
      return (
        <div key={c.id}>
          <div className={"row card" + (sel === "c:" + c.id ? " sel" : "")} style={{ paddingLeft: 20 }} onClick={() => setSel("c:" + c.id)}>
            <span className="cell"><I n="card" /></span>
            <span className="grow clip">{c.title}</span>
            <span className="est">{c.est}</span>
            <ProjTag p={c.proj} />
            <span className="plus" title="New task on this card" onClick={stop(() => addTask(c.id))}><I n="plus" /></span>
          </div>
          {tasks.map((s) => <Row key={s.id} s={s} depth={2} />)}
        </div>
      );
    })}
    {!list.length && <div className="row dim" style={{ paddingLeft: 42 }}>{empty || "no cards"}</div>}
  </>;
  const Grp = ({ k, label, count, children }) => <>
    <div className="row grp" style={{ paddingLeft: 20 }} onClick={() => tog(k)}><Chev open={!fold[k]} /><span>{label}</span><span className="cnt">{count}</span></div>
    {!fold[k] && children}
  </>;
  return (
    <aside className="sidebar">
      <div className="titlebar">
        <span className="lights"><i></i><i></i><i></i></span>
        <span className="grow"></span>
        <span className="tb-ic" title="Settings"><I n="gear" /></span>
        <span ref={tbPlus} className="tb-ic" title="New" onClick={() => setTbMenu(!tbMenu)}><I n="plus" /></span>
        {tbMenu && <Menu anchor={tbPlus.current} close={() => setTbMenu(false)} items={[["New session"], ["New project"], [variant === "b" ? "New Shortcut segment" : "New Shortcut section", () => addSource()]]} />}
      </div>
      <Head k="active" label="Active" count={needs} />
      {!fold.active && active.map((s) => <Row key={s.id} s={s} depth={0} tag={codeOf(s)} />)}
      {!fold.active && activeCards && doingCards.map((c) => {
        const k = "ac:" + c.id, open = !fold[k], tasks = SLICES.filter((s) => s.card === c.id && s.state !== "done").sort((a, b) => (launched(b) ? 1 : 0) - (launched(a) ? 1 : 0));
        return (
          <div key={c.id}>
            <div className={"row card" + (sel === "c:" + c.id ? " sel" : "")} onClick={() => setSel("c:" + c.id)}>
              <span className="cell fold" onClick={stop(() => tog(k))}><I n="shortcut" size={13} /></span>
              <ProjTag p={c.proj} />
              <span className="grow clip">{c.title}</span>
              <span className="mono xs dimtxt">{c.id}</span>
              <span className="plus" title="New task on this card" onClick={stop(() => addTask(c.id))}><I n="plus" /></span>
            </div>
            {open && tasks.map((s) => <Row key={s.id} s={s} depth={1} />)}
          </div>
        );
      })}
      <div className="grow scroll" ref={scrollRef} style={{ position: "relative" }}>
        <Head k="work" label="Projects" right={<span className="plus" title="New project"><I n="folderNew" /></span>} />
        {!fold.work && PROJECTS.map((p) => {
          const open = !fold["p:" + p.id], rows = SLICES.filter((s) => s.project === p.id);
          let lastMs = null;
          return (
            <div key={p.id}>
              <div className="row proj" onClick={() => tog("p:" + p.id)}>
                <span className="cell fold"><I n={open ? "folder" : "folder"} /></span>
                <span className="grow clip">{p.name}</span>
                {open && <span className="plus" title="New milestone" onClick={stop()}><I n="plus" /></span>}
              </div>
              {open && rows.map((s) => {
                const msRow = s.ms !== lastMs ? <div key={"ms" + s.ms} className="row ms" style={{ paddingLeft: 20 }}><span className="cell"><I n="ms" /></span><span className="grow clip">{s.ms}</span><span className="cnt">{s.msCount}</span></div> : null;
                lastMs = s.ms;
                return <React.Fragment key={s.id}>{msRow}<Row s={s} depth={2} /></React.Fragment>;
              })}
            </div>
          );
        })}
        {variant === "b" ? (
          <div>
            <div className="row head src" onClick={() => tog("sc")}>
              <Chev open={!fold.sc} />
              <span className="cell" style={{ color: "var(--ink-2)" }}><I n="shortcut" /></span>
              <span className="grow clip">Shortcut</span>
              <span ref={scDots} className="plus" style={{ display: "flex" }} onClick={stop(() => setScMenu(!scMenu))}><I n="dots" /></span>
              {scMenu && <Menu anchor={scDots.current} close={() => setScMenu(false)} items={[["New ready segment", () => addSource()], ["Refresh"]]} />}
            </div>
            {!fold.sc && <>
              {!activeCards && <Grp k="b:doing" label="Doing" count={CARDS.filter((c) => c.group === "doing").length}><CardRows list={CARDS.filter((c) => c.group === "doing")} empty="nothing in progress" /></Grp>}
              {sources.map((src) => {
                const list = CARDS.filter((c) => c.group === "ready" && matches(c, src.filters)), sk = "seg:" + src.id;
                return (
                  <div key={src.id}>
                    <SourceHead seg src={src} open={!fold[sk]} onTog={() => tog(sk)} onRename={(name) => upd(src.id, { name })} onFilters={(filters) => upd(src.id, { filters })} onDuplicate={() => addSource(src)} onRemove={() => setSources(sources.filter((x) => x.id !== src.id))} />
                    {!fold[sk] && <CardRows list={list} empty="no ready cards match" />}
                  </div>
                );
              })}
              <Grp k="b:done" label="Done" count={DONE_CARDS}><div className="row dim" style={{ paddingLeft: 42 }}>loads on expand</div></Grp>
            </>}
          </div>
        ) : sources.map((src) => {
          const cards = CARDS.filter((c) => matches(c, src.filters)), sk = "src:" + src.id, sopen = !fold[sk];
          return (
            <div key={src.id}>
              <SourceHead src={src} open={sopen} onTog={() => tog(sk)} onRename={(name) => upd(src.id, { name })} onFilters={(filters) => upd(src.id, { filters })} onDuplicate={() => addSource(src)} onRemove={() => setSources(sources.filter((x) => x.id !== src.id))} />
              {sopen && groups.filter(([g]) => !(activeCards && g === "doing")).map(([g, label]) => {
                const list = cards.filter((c) => c.group === g), gk = sk + ":" + g, open = !fold[gk];
                return (
                  <div key={g}>
                    <div className="row grp" style={{ paddingLeft: 20 }} onClick={() => tog(gk)}><Chev open={open} /><span>{label}</span><span className="cnt">{list.length}</span></div>
                    {open && list.map((c) => {
                      const tasks = SLICES.filter((s) => s.card === c.id);
                      return (
                        <div key={c.id}>
                          <div className={"row card" + (sel === "c:" + c.id ? " sel" : "")} style={{ paddingLeft: 20 }} onClick={() => setSel("c:" + c.id)}>
                            <span className="cell"><I n="card" /></span>
                            <span className="grow clip">{c.title}</span>
                            <span className="est">{c.est}</span>
                            <ProjTag p={c.proj} />
                            <span className="plus" title="New task on this card" onClick={stop(() => addTask(c.id))}><I n="plus" /></span>
                          </div>
                          {tasks.map((s) => <Row key={s.id} s={s} depth={2} />)}
                        </div>
                      );
                    })}
                    {open && !list.length && <div className="row dim" style={{ paddingLeft: 42 }}>no cards</div>}
                  </div>
                );
              })}
              {sopen && <div className="row grp" style={{ paddingLeft: 20 }} onClick={() => tog(sk + ":done")}><Chev open={fold[sk + ":done"] === false} /><span>Done</span><span className="cnt">{src.id === "mine" ? DONE_CARDS : 9}</span></div>}
            </div>
          );
        })}
      </div>
      <hr />
      <Head k="scratch" label="Scratch" right={<span className="plus"><I n="plus" /></span>} />
    </aside>
  );
}

function StatusBar({ s, c }) {
  const running = SLICES.filter((x) => x.state === "working").length;
  return (
    <footer className="status">
      <span className="logo"><I n="logo" size={15} /></span>
      <span>{running} agent{running === 1 ? "" : "s"}</span>
      <span className="sep">|</span><span>5h: 19% (1:30pm)</span>
      <span className="sep">|</span><span>week: 27% (Friday)</span>
      <span className="grow"></span>
      {s && <><span>{s.card ? "sc-" + s.card : parentOf(s).name}</span><span className="sep">/</span><span>{s.title}</span></>}
      {c && <><span>{SC_PROJECTS[c.proj][1]}</span><span className="sep">/</span><span>sc-{c.id}</span></>}
    </footer>
  );
}
Object.assign(window, { I, ProjTag, Dot, Chev, Sidebar, StatusBar });

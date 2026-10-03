const { Icon: SCIcon } = DS;

const SC_PEOPLE = { mk: ["MK", "Maya Kern", "var(--system-pink)"], jt: ["JT", "Jonas Tran", "var(--system-blue)"], pd: ["PD", "Priya Deol", "var(--system-teal)"], dw: ["DW", "Dana Wolfe", "var(--system-orange)"] };

function SCAvatar({ p, size = 18, active, dim }) {
  const [ini, name, tint] = SC_PEOPLE[p];
  return (
    <span title={name} style={{ width: size, height: size, borderRadius: size / 2, flexShrink: 0, display: "inline-flex", alignItems: "center", justifyContent: "center", font: `600 ${Math.round(size * 0.44)}px/1 var(--font-system)`, color: "var(--accent-text)", background: tint, opacity: dim ? 0.4 : 1, boxShadow: active ? "0 0 0 1.5px var(--window-bg), 0 0 0 3px var(--accent)" : "0 0 0 1.5px var(--window-bg)" }}>{ini}</span>
  );
}

function SCAvatarStack({ ids, working = [] }) {
  return (
    <span style={{ display: "inline-flex", flexDirection: "row-reverse", paddingLeft: 4 }}>
      {[...ids].reverse().map((p) => <span key={p} style={{ marginLeft: -5 }}><SCAvatar p={p} active={working.includes(p)} dim={working.length > 0 && !working.includes(p)} /></span>)}
    </span>
  );
}

const SC_PROJECTS = { na: ["NA", "Native App", "var(--system-blue)"], bd: ["BD", "Board", "var(--system-teal)"], se: ["S", "Search", "var(--system-pink)"] };

function SCProjBadge({ proj }) {
  const [short, name, tint] = SC_PROJECTS[proj];
  return <span title={name} style={{ display: "inline-flex", alignItems: "center", justifyContent: "center", minWidth: 18, height: 16, padding: "0 3px", borderRadius: 4, flexShrink: 0, font: "600 10px/1 var(--font-system)", color: tint, background: `color-mix(in srgb, ${tint} 20%, transparent)` }}>{short}</span>;
}

const SC = {
  mine: [
    { id: "c-diff", title: "Improve diff review ergonomics", proj: "na", people: ["mk", "jt"], slices: [
      { id: "t-comments", title: "Diff comments reach the agent", g: "claimed", st: ["Working", "var(--system-orange)"] },
      { id: "t-syntax", title: "Syntax highlighting in the diff", g: "review", st: ["+368 −17", "var(--system-green)"] },
      { id: "t-hunks", title: "Collapse unchanged hunks", g: "todo" }
    ] }
  ],
  doing: [
    { id: "c-mouse", title: "Board mouse support", proj: "bd", people: ["pd"], prs: [
      { id: "p-drag", num: "#412", title: "Drag cards between columns", state: "open" },
      { id: "p-hit", num: "#407", title: "Widen board hit targets", state: "merged" }
    ] },
    { id: "c-search", title: "Story search across workspaces", proj: "se", people: ["jt", "dw"], prs: [
      { id: "p-index", num: "#415", title: "Index stories per workspace", state: "open" }
    ] }
  ],
  ready: [
    { id: "c-kanban", title: "Kanban column view", proj: "bd", est: "3", people: ["pd"] },
    { id: "c-wheel", title: "Wheel scrolling in the Active panel", proj: "bd", est: "1", people: [] },
    { id: "c-storage", title: "Pluggable plan storage", proj: "na", est: "5", people: ["jt"] },
    { id: "c-wishlist", title: "Wishlist triage flow", proj: "na", est: "2", people: [] }
  ]
};

function SCFilterBar() {
  const pill = (label, on) => (
    <span key={label} style={{ display: "inline-flex", alignItems: "center", gap: 4, height: 22, padding: "0 9px", borderRadius: 11, border: on ? "1px solid color-mix(in srgb, var(--accent) 55%, transparent)" : "1px solid var(--control-border)", background: on ? "color-mix(in srgb, var(--accent) 18%, transparent)" : "transparent", font: "var(--font-caption1)", color: on ? "var(--label)" : "var(--label-secondary)", whiteSpace: "nowrap" }}>
      {label}
      <SCIcon name={on ? "xmark" : "chevron_down"} size={8} weight={700} color="var(--label-tertiary)" style={{ verticalAlign: 0 }} />
    </span>
  );
  return (
    <div style={{ padding: "0 0 12px" }}>
      <div style={{ display: "flex", alignItems: "center", gap: 7, height: 26, padding: "0 9px", borderRadius: 6, background: "var(--field-bg)", border: "0.5px solid var(--control-border)", marginBottom: 8 }}>
        <SCIcon name="search" size={12} color="var(--label-tertiary)" style={{ verticalAlign: 0 }} />
        <span style={{ font: "var(--font-body)", color: "var(--label-tertiary)" }}>Filter cards</span>
      </div>
      <div style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
        {pill("Owner: Maya", true)}
        {pill("Epic: Any")}
        {pill("Label: Any")}
        {pill("Type: Any")}
        <span style={{ display: "inline-flex", alignItems: "center", gap: 4, height: 22, padding: "0 9px", borderRadius: 11, font: "var(--font-caption1)", color: "var(--label-tertiary)" }}><SCIcon name="plus" size={9} weight={700} color="var(--label-tertiary)" style={{ verticalAlign: 0 }} />Filter</span>
      </div>
    </div>
  );
}

const SCG = { todo: ["circle", "var(--label-tertiary)"], claimed: ["circle_lefthalf_fill", "var(--system-orange)"], done: ["checkmark_circle", "var(--system-green)"], blocked: ["nosign", "var(--label-tertiary)"], review: ["checkmark_seal", "var(--system-green)"] };

function SCSliceRow({ t, selectedId }) {
  const sel = t.id === selectedId;
  const [ic, c] = SCG[t.g];
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 8, height: 29, margin: "0 -6px", padding: "0 14px 0 32px", borderRadius: 7, background: sel ? "color-mix(in srgb, var(--accent) 24%, transparent)" : "transparent", color: "var(--label)" }}>
      <span style={{ width: 13, flexShrink: 0, display: "inline-flex", justifyContent: "center" }}><SCIcon name={ic} size={12} color={c} style={{ verticalAlign: 0 }} /></span>
      <span style={{ flex: 1, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{t.title}</span>
      {t.st && <span className={t.g === "claimed" ? "ws-pulse" : ""} style={{ font: "var(--font-caption1)", color: t.st[1], flexShrink: 0, fontVariantNumeric: "tabular-nums" }}>{t.st[0]}</span>}
    </div>
  );
}

function SCPrRow({ p }) {
  const merged = p.state === "merged";
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 8, height: 29, margin: "0 -6px", padding: "0 14px 0 32px", color: "var(--label-secondary)" }}>
      <span style={{ width: 13, flexShrink: 0, display: "inline-flex", justifyContent: "center" }}><SCIcon name="arrow_branch" size={12} color={merged ? "var(--system-pink)" : "var(--system-green)"} style={{ verticalAlign: 0 }} /></span>
      <span style={{ font: "var(--font-subheadline)", color: "var(--label-tertiary)", fontVariantNumeric: "tabular-nums", flexShrink: 0 }}>{p.num}</span>
      <span style={{ flex: 1, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{p.title}</span>
      <span style={{ font: "var(--font-caption1)", color: merged ? "var(--system-pink)" : "var(--system-green)", flexShrink: 0 }}>{merged ? "Merged" : "Open"}</span>
    </div>
  );
}

function SCKids({ children, x = 11 }) {
  return (
    <div style={{ position: "relative" }}>
      <span style={{ position: "absolute", left: x, top: 2, bottom: 2, width: 1, background: "var(--separator)" }}></span>
      {children}
    </div>
  );
}

function SCMineCard({ c, selectedId }) {
  return (
    <React.Fragment>
      <div style={{ display: "flex", alignItems: "center", gap: 8, height: 32, margin: "0 -6px", padding: "0 14px 0 11px" }}>
        <SCIcon name="rectangle_fill_on_rectangle_angled_fill" size={13} color="var(--accent)" style={{ verticalAlign: 0 }} />
        <span style={{ font: "var(--font-body-emphasized)", flex: 1, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis", color: "var(--label)" }}>{c.title}</span>
        <SCProjBadge proj={c.proj} />
        <SCAvatarStack ids={c.people} working={["mk"]} />
      </div>
      <SCKids>{c.slices.map((t) => <SCSliceRow key={t.id} t={t} selectedId={selectedId} />)}</SCKids>
    </React.Fragment>
  );
}

function SCDoingCard({ c }) {
  return (
    <React.Fragment>
      <div style={{ display: "flex", alignItems: "center", gap: 8, height: 32, margin: "0 -6px", padding: "0 14px 0 11px" }}>
        <SCIcon name="rectangle_on_rectangle_angled" size={13} color="var(--label-secondary)" style={{ verticalAlign: 0 }} />
        <span style={{ font: "var(--font-body)", flex: 1, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis", color: "var(--label)" }}>{c.title}</span>
        <SCProjBadge proj={c.proj} />
        <SCAvatarStack ids={c.people} />
      </div>
      <SCKids>
        {c.prs.map((p) => <SCPrRow key={p.id} p={p} />)}
        {c.prs.length === 0 && <div style={{ display: "flex", alignItems: "center", height: 26, margin: "0 -6px", padding: "0 14px 0 32px", font: "var(--font-subheadline)", color: "var(--label-tertiary)" }}>No PRs yet</div>}
      </SCKids>
    </React.Fragment>
  );
}

function SCReadyCard({ c }) {
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 8, height: 30, margin: "0 -6px", padding: "0 14px 0 11px" }}>
      <SCIcon name="rectangle_on_rectangle_angled" size={13} color="var(--label-tertiary)" style={{ verticalAlign: 0 }} />
      <span style={{ font: "var(--font-body)", flex: 1, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis", color: "var(--label)" }}>{c.title}</span>
      <SCProjBadge proj={c.proj} />
      <span style={{ font: "var(--font-caption1)", color: "var(--label-tertiary)", fontVariantNumeric: "tabular-nums", border: "1px solid var(--control-border)", borderRadius: 4, padding: "0 4px", lineHeight: "14px" }}>{c.est}</span>
      {c.people.length > 0 && <SCAvatarStack ids={c.people} />}
    </div>
  );
}

function SCSectionHead({ icon, label, count, v = "a", open = true }) {
  const chev = <SCIcon name={open ? "chevron_down" : "chevron_right"} size={10} weight={700} color="var(--label-tertiary)" style={{ verticalAlign: 0 }} />;
  const ic = (sz = 11) => <SCIcon name={icon} size={sz} color="var(--label-tertiary)" style={{ verticalAlign: 0 }} />;
  const lbl = <span style={{ font: "600 11px/14px var(--font-system)", color: "var(--label-tertiary)", letterSpacing: ".4px" }}>{label}</span>;
  const cnt = count != null && <span style={{ font: "var(--font-caption1)", color: "var(--label-tertiary)", fontVariantNumeric: "tabular-nums" }}>{count}</span>;
  if (v === "b") return (
    <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "0 8px 6px 5px" }}>
      <span style={{ width: 13, flexShrink: 0, display: "inline-flex", justifyContent: "center" }}>{ic()}</span>
      {lbl}
      <span style={{ display: "inline-flex", marginLeft: -3 }}>{chev}</span>
      <span style={{ flex: 1 }}></span>
      {cnt}
    </div>
  );
  if (v === "c") return (
    <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "0 8px 6px 5px" }}>
      <span style={{ width: 13, flexShrink: 0, display: "inline-flex", justifyContent: "center" }}>{ic()}</span>
      {lbl}
      <span style={{ flex: 1 }}></span>
      {cnt}
      {chev}
    </div>
  );
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "0 8px 6px 5px" }}>
      <span style={{ width: 13, flexShrink: 0, display: "inline-flex", justifyContent: "center" }}>{chev}</span>
      {ic()}
      {lbl}
      <span style={{ flex: 1 }}></span>
      {cnt}
    </div>
  );
}

function SCRail({ selectedId, headV = "a" }) {
  return (
    <div style={{ width: 372, flexShrink: 0, overflowY: "auto", borderRight: "0.5px solid var(--separator)", padding: "14px 18px 16px" }}>
      <SCFilterBar />
      <SCSectionHead icon="bolt" label="ACTIVE" count={SC.mine.length} v={headV} />
      {SC.mine.map((c) => <SCMineCard key={c.id} c={c} selectedId={selectedId} />)}
      <div style={{ borderBottom: "0.5px solid var(--separator)", margin: "12px 0" }}></div>
      <SCSectionHead icon="person_2" label="DOING" count={SC.doing.length} v={headV} />
      {SC.doing.map((c) => <SCDoingCard key={c.id} c={c} />)}
      <div style={{ borderBottom: "0.5px solid var(--separator)", margin: "12px 0" }}></div>
      <SCSectionHead icon="tray" label="READY" count={SC.ready.length} v={headV} />
      {SC.ready.map((c) => <SCReadyCard key={c.id} c={c} />)}
      <div style={{ borderBottom: "0.5px solid var(--separator)", margin: "12px 0" }}></div>
      <SCSectionHead icon="checkmark_circle" label="DONE" count={36} v={headV} open={false} />
    </div>
  );
}

function SCHeadVariants() {
  const panel = (v, title, note) => (
    <div key={v} style={{ width: 340 }}>
      <div style={{ font: "600 12px/16px var(--font-system)", color: "#71717a", marginBottom: 2 }}>{title}</div>
      <div style={{ font: "var(--font-caption1)", color: "#a1a1aa", marginBottom: 8 }}>{note}</div>
      <div style={{ background: "var(--window-bg)", border: "0.5px solid var(--separator)", borderRadius: 10, padding: "12px 18px 10px" }}>
        <SCSectionHead icon="person_2" label="DOING" count={2} v={v} />
        <SCDoingCard c={SC.doing[0]} />
        <div style={{ borderBottom: "0.5px solid var(--separator)", margin: "10px 0" }}></div>
        <SCSectionHead icon="tray" label="READY" count={4} v={v} open={false} />
      </div>
    </div>
  );
  return (
    <div className="nat3" style={{ display: "flex", gap: 28, alignItems: "flex-start" }}>
      {panel("a", "A — chevron leads", "Chevron in the alignment slot, icon beside the label")}
      {panel("b", "B — chevron trails label", "Icon keeps the slot, small chevron hangs off the label")}
      {panel("c", "C — chevron at right edge", "Icon keeps the slot, chevron after the count, list-style")}
    </div>
  );
}

function SCMeta() {
  const G = ({ label, children }) => (
    <div style={{ marginBottom: 26 }}>
      <div style={{ font: "600 11px/14px var(--font-system)", color: "var(--label-tertiary)", letterSpacing: ".4px", marginBottom: 8 }}>{label}</div>
      {children}
    </div>
  );
  const chip = (t) => <span key={t} style={{ font: "var(--font-caption1)", color: "var(--label-secondary)", border: "1px solid var(--control-border)", borderRadius: 10, padding: "2px 8px" }}>{t}</span>;
  return (
    <div style={{ width: 300, flexShrink: 0, padding: "44px 28px 0", borderLeft: "0.5px solid var(--separator)" }}>
      <G label="STATE"><span style={{ display: "inline-flex", alignItems: "center", gap: 7, font: "var(--font-body)", color: "var(--label)" }}><span style={{ color: "var(--system-orange)", fontSize: 8 }}>●</span>Doing</span></G>
      <G label="OWNERS">
        <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
          {["mk", "jt"].map((p) => <span key={p} style={{ display: "inline-flex", alignItems: "center", gap: 8, font: "var(--font-body)", color: "var(--label)" }}><SCAvatar p={p} active={p === "mk"} />{SC_PEOPLE[p][1]}{p === "mk" && <span style={{ font: "var(--font-caption1)", color: "var(--system-orange)" }}>working now</span>}</span>)}
        </div>
      </G>
      <G label="CARD"><span style={{ font: "var(--font-body)", color: "var(--label)" }}>Improve diff review ergonomics</span></G>
      <G label="PROJECT"><span style={{ display: "inline-flex", alignItems: "center", gap: 8, font: "var(--font-body)", color: "var(--label)" }}><SCProjBadge proj="na" />Native App</span></G>
      <G label="EPIC"><span style={{ font: "var(--font-body)", color: "var(--label-secondary)" }}>Native app parity</span></G>
      <G label="LABELS"><div style={{ display: "flex", gap: 6, flexWrap: "wrap" }}>{["diff", "agent"].map(chip)}</div></G>
      <G label="ESTIMATE"><span style={{ font: "var(--font-body)", color: "var(--label-secondary)" }}>3 points</span></G>
      <G label="REQUESTER"><span style={{ display: "inline-flex", alignItems: "center", gap: 8, font: "var(--font-body)", color: "var(--label-secondary)" }}><SCAvatar p="dw" size={16} />Dana Wolfe</span></G>
    </div>
  );
}

function SCShell() {
  return (
    <div className="nat3">
      <MacWindow width={1440} height={880}>
        <U3Header />
        <div style={{ display: "flex", flex: 1, minHeight: 0 }}>
          <SCRail selectedId="t-comments" />
          <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column" }}>
            <U3PaneHeader ms="Improve diff review ergonomics" title="Diff comments reach the agent" tab="Brief" />
            <div style={{ flex: 1, minHeight: 0, display: "flex" }}>
              <div style={{ flex: 1, minWidth: 0, overflowY: "auto", padding: "0 40px" }}>
                <div style={{ maxWidth: 980, margin: "36px auto 0", background: "var(--card-bg)", border: "0.5px solid var(--separator)", borderRadius: 10, padding: "24px 30px 28px" }}>
                  <div style={{ display: "flex", alignItems: "baseline", marginBottom: 18 }}>
                    <span style={{ font: "var(--font-body-emphasized)", color: "var(--label)", flex: 1 }}>Brief</span>
                    <span style={{ font: "var(--font-body)", color: "var(--label-tertiary)" }}>Edit…</span>
                  </div>
                  <div style={{ font: "400 15px/24px var(--font-system)", color: "var(--label-secondary)", textWrap: "pretty" }}>
                    Comments left on a diff in the app never reach the working agent — they queue in the review pane until the session ends. Wire them through: a comment posted on any hunk lands in the agent's session as a user turn (quoting the file, line range, and comment body), the agent acknowledges it in the transcript, and resolving the comment in the diff marks the turn addressed. Acceptance: a comment posted mid-session shows up in the transcript within a second, and resolving it updates both panes.
                  </div>
                </div>
              </div>
              <SCMeta />
            </div>
            <U3Footer />
          </div>
        </div>
        <U3Progress />
      </MacWindow>
    </div>
  );
}

Object.assign(window, { SC, SCShell, SCRail, SCMeta, SCAvatar, SCAvatarStack, SCFilterBar, SCProjBadge, SCHeadVariants });

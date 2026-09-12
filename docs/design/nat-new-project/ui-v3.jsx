const { Icon: U3Icon, TrafficLights: U3TL, ToolbarButton: U3TBBtn } = DS;

const U3 = {
  active: [["s-scope", "Scope the planning agent to its project", "Working", "var(--system-orange)", "<1m", "M38: App fixes", null]],
  ms: [
    { id: "M31", title: "M31: App parity", done: 1, total: 4, slices: [
      ["todo", "s-stuck", "Release a stuck slice from the app"],
      ["todo", "s-wish", "Show the wishlist in the app"],
      ["todo", "s-projcmd", "Add project-list and project-open commands"]
    ] },
    { id: "M32", title: "M32: Review flow", done: 0, total: 2, collapsed: true },
    { id: "M33", title: "M33: Plan commands & Notion cleanup", done: 4, total: 10, slices: [
      ["todo", "s-cycles", "Refuse only the cycles a plan itself takes part in"],
      ["todo", "s-statusprop", "Drop the status property type, and keep the text"],
      ["todo", "s-migration", "Remove the migration path"],
      ["todo", "s-depth", "Raise the block depth cap for deeply nested plans"],
      ["todo", "s-seedwish", "Seed a Wishlist section on new project pages"],
      ["todo", "s-msmove", "Add a milestone-move command"]
    ] },
    { id: "M34", title: "M34: Completion checks", done: 0, total: 4, collapsed: true },
    { id: "M35", title: "M35: Docs & versioning", done: 0, total: 2, collapsed: true },
    { id: "M36", title: "M36: Pluggable plan storage", done: 4, total: 6, slices: [
      ["blocked", "s-mirror", "Mirror a local project into Notion"],
      ["todo", "s-backend", "Choose the backend per project"]
    ] },
    { id: "M37", title: "M37: Agent pane fidelity", done: 1, total: 3, slices: [
      ["todo", "s-kill", "Kill an agent session from the app once it goes idle"],
      ["todo", "s-shiftenter", "Forward shift+enter and wire link clicks in the pane"]
    ] },
    { id: "M38", title: "M38: App fixes", done: 2, total: 14, current: true, slices: [
      ["todo", "s-interrupt", "Drop the Interrupt and Open in Terminal buttons"],
      ["todo", "s-embedded", "Always run the embedded nat, and refuse to launch"],
      ["todo", "s-skeleton", "Line the skeletons up with the content they stand for"],
      ["todo", "s-pin", "Pin the rail's in-flight sections above the fold"],
      ["todo", "s-emptied", "Drop an emptied milestone from the rail's todo list"]
    ] }
  ]
};
const U3G = { todo: ["circle", "var(--label-tertiary)"], claimed: ["circle_lefthalf_fill", "var(--system-orange)"], done: ["checkmark_circle", "var(--system-green)"], blocked: ["nosign", "var(--label-tertiary)"], review: ["checkmark_seal", "var(--system-green)"] };

const U3_PROJECTS = [
  ["notion-agent-tracker", "var(--accent)", 14, true],
  ["Property market search", "var(--system-red)", 0, false],
  ["Homelab", "var(--system-orange)", 0, false]
];

function U3Header() {
  return (
    <div style={{ display: "flex", alignItems: "stretch", height: 44, background: "var(--strip-bg)", flexShrink: 0 }}>
      <div style={{ display: "flex", alignItems: "center", padding: "0 16px" }}><U3TL /></div>
      <div style={{ display: "flex", alignItems: "flex-end" }}>
        {U3_PROJECTS.map(([name, tint, count, on], i) => (
          <React.Fragment key={name}>
            <div style={{ display: "flex", alignItems: "center", gap: 8, height: 38, boxSizing: "border-box", padding: "0 14px 4px 18px", minWidth: on ? 200 : 150, maxWidth: 250, borderRadius: "10px 10px 0 0", background: on ? "var(--window-bg)" : "transparent", position: "relative" }}>
              {on && <><span style={{ position: "absolute", left: -10, bottom: 0, width: 10, height: 10, background: "radial-gradient(circle at 0 0, transparent 10px, var(--window-bg) 10.5px)" }}></span><span style={{ position: "absolute", right: -10, bottom: 0, width: 10, height: 10, background: "radial-gradient(circle at 100% 0, transparent 10px, var(--window-bg) 10.5px)" }}></span></>}
              <span style={{ color: tint, fontSize: 8 }}>●</span>
              <span style={{ font: on ? "600 12px/15px var(--font-system)" : "var(--font-subheadline)", color: on ? "var(--label)" : "var(--label-secondary)", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis", flex: 1 }}>{name}</span>
              {on && count > 0 && <span style={{ font: "var(--font-caption2)", fontVariantNumeric: "tabular-nums", color: "var(--label-secondary)", background: "var(--label-quaternary)", borderRadius: 8, padding: "0 7px", lineHeight: "15px" }}>{count}</span>}
              {on && <U3Icon name="xmark" size={10} weight={600} color="var(--label-tertiary)" style={{ verticalAlign: 0 }} />}
            </div>
            {!on && !U3_PROJECTS[i + 1]?.[3] && <span style={{ width: 1, height: 16, background: "var(--label-quaternary)", alignSelf: "center", marginBottom: 4 }}></span>}
          </React.Fragment>
        ))}
        <div title="New Project Tab" style={{ display: "flex", alignItems: "center", justifyContent: "center", width: 30, height: 30, borderRadius: 7, alignSelf: "center", marginLeft: 10 }}>
          <U3Icon name="plus" size={14} color="var(--label-secondary)" style={{ verticalAlign: 0 }} />
        </div>
      </div>
      <div style={{ display: "flex", alignItems: "center", gap: 14, padding: "0 16px", marginLeft: "auto" }}>
        <span style={{ font: "var(--font-subheadline)", color: "var(--label-tertiary)", fontVariantNumeric: "tabular-nums" }}>187/233 slices</span>
        <U3TBBtn icon="plus_rectangle_on_rectangle" title="New Slice" />
        <U3TBBtn icon="wand_stars" title="Workshop the Plan" />
      </div>
    </div>
  );
}

function U3SectionHead({ icon, label, pad = "0 8px 6px 5px" }) {
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 8, padding: pad }}>
      <span style={{ width: 13, flexShrink: 0, display: "inline-flex", justifyContent: "center" }}><U3Icon name={icon} size={11} color="var(--label-tertiary)" style={{ verticalAlign: 0 }} /></span>
      <span style={{ font: "600 11px/14px var(--font-system)", color: "var(--label-tertiary)", letterSpacing: ".4px" }}>{label}</span>
    </div>
  );
}

function U3ActiveRow({ s, selected }) {
  const [id, n, st, tint, elapsed, msT, detail] = s;
  return (
    <div style={{ display: "flex", gap: 8, margin: "0 -6px", padding: "6px 14px 7px 11px", borderRadius: 7, background: selected ? "color-mix(in srgb, var(--accent) 24%, transparent)" : "transparent" }}>
      <span style={{ width: 13, flexShrink: 0, display: "inline-flex", justifyContent: "center" }}><span className={selected ? "" : "ws-pulse"} style={{ color: tint, fontSize: 9, lineHeight: "19px" }}>●</span></span>
      <span style={{ flex: 1, minWidth: 0 }}>
        <span style={{ display: "flex", alignItems: "baseline", gap: 8 }}>
          <span style={{ flex: 1, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis", color: "var(--label)" }}>{n}</span>
          <span style={{ font: "var(--font-subheadline)", fontVariantNumeric: "tabular-nums", color: "var(--label-tertiary)" }}>{elapsed}</span>
        </span>
        <span style={{ display: "flex", gap: 5, marginTop: 1, font: "var(--font-subheadline)", color: "var(--label-tertiary)", whiteSpace: "nowrap", overflow: "hidden" }}>
          <span style={{ color: tint, flexShrink: 0 }}>{st}</span>
          <span>·</span>
          <span style={{ overflow: "hidden", textOverflow: "ellipsis" }}>{msT}</span>
          {detail && <><span>·</span><span style={{ overflow: "hidden", textOverflow: "ellipsis" }}>{detail}</span></>}
        </span>
      </span>
    </div>
  );
}

function U3SliceRow({ s, selectedId }) {
  const [g, id, n] = [s[0], s[1], s[2]];
  const [ic, c] = U3G[g];
  const sel = id === selectedId;
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 8, height: 29, margin: "0 -6px", padding: "0 14px 0 32px", borderRadius: 7, background: sel ? "color-mix(in srgb, var(--accent) 24%, transparent)" : "transparent", color: g === "blocked" ? "var(--label-tertiary)" : "var(--label)" }}>
      <span style={{ width: 13, flexShrink: 0, display: "inline-flex", justifyContent: "center" }}><U3Icon name={ic} size={12} color={c} style={{ verticalAlign: 0 }} /></span>
      <span style={{ flex: 1, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{n}</span>
    </div>
  );
}

function U3MsFolder({ m, selectedId }) {
  const open = !m.collapsed;
  return (
    <React.Fragment>
      <div style={{ display: "flex", alignItems: "center", gap: 8, height: 30, margin: "0 -6px", padding: "0 14px 0 11px" }}>
        <span style={{ width: 13, flexShrink: 0, display: "inline-flex", justifyContent: "center" }}><U3Icon name={open ? "chevron_down" : "chevron_right"} size={10} weight={700} color="var(--label-tertiary)" style={{ verticalAlign: 0 }} /></span>
        <U3Icon name={open ? "folder_fill" : "folder"} size={13} color={m.current ? "var(--accent)" : open ? "var(--label-secondary)" : "var(--label-tertiary)"} style={{ verticalAlign: 0 }} />
        <span style={{ font: m.current ? "var(--font-body-emphasized)" : "var(--font-body)", flex: 1, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis", color: "var(--label)" }}>{m.title}</span>
        <span style={{ font: "var(--font-subheadline)", color: "var(--label-tertiary)", fontVariantNumeric: "tabular-nums" }}>{m.done}/{m.total}</span>
      </div>
      {open && (m.slices || []).map((s) => <U3SliceRow key={s[1]} s={s} selectedId={selectedId} />)}
    </React.Fragment>
  );
}

function U3Rail({ selectedId, data = U3 }) {
  return (
    <div style={{ width: 372, flexShrink: 0, overflowY: "auto", borderRight: "0.5px solid var(--separator)", padding: "14px 18px 16px" }}>
      <U3SectionHead icon="bolt" label="ACTIVE" />
      {data.active.map((s) => <U3ActiveRow key={s[0]} s={s} selected={s[0] === selectedId} />)}
      <div style={{ borderBottom: "0.5px solid var(--separator)", margin: "12px 0" }}></div>
      <U3SectionHead icon="list_bullet" label="TODO" pad="0 8px 8px 5px" />
      {data.ms.map((m) => <U3MsFolder key={m.id} m={m} selectedId={selectedId} />)}
    </div>
  );
}

function U3Tabs({ value }) {
  const tabs = [["checkmark_seal_fill", "Brief"], ["circle", "Agent"], ["circle", "Diff"], ["circle", "PR"]];
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 34, flexShrink: 0 }}>
      {tabs.map(([ic, label]) => {
        const on = label === value;
        return on ? (
          <span key={label} style={{ display: "inline-flex", alignItems: "center", gap: 6, height: 28, padding: "0 12px", borderRadius: 14, background: "color-mix(in srgb, var(--accent) 26%, transparent)" }}>
            <U3Icon name={ic} size={13} color="var(--accent)" style={{ verticalAlign: 0 }} />
            <span style={{ font: "600 13px/16px var(--font-system)", color: "var(--label)" }}>{label}</span>
          </span>
        ) : (
          <span key={label} style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
            <U3Icon name={ic} size={11} color="var(--label-tertiary)" style={{ verticalAlign: 0 }} />
            <span style={{ font: "var(--font-body)", color: "var(--label-secondary)" }}>{label}</span>
          </span>
        );
      })}
    </div>
  );
}

function U3PaneHeader({ ms, title, tab }) {
  return (
    <div style={{ display: "flex", alignItems: "flex-end", gap: 24, padding: "14px 24px 12px", borderBottom: "0.5px solid var(--separator)", flexShrink: 0 }}>
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ font: "var(--font-subheadline)", color: "var(--label-tertiary)", marginBottom: 3 }}>{ms}</div>
        <div style={{ font: "600 19px/24px var(--font-system)", color: "var(--label)", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{title}</div>
      </div>
      <div style={{ paddingBottom: 4 }}><U3Tabs value={tab} /></div>
    </div>
  );
}

function U3BriefCard() {
  return (
    <div style={{ maxWidth: 980, margin: "36px auto 0", background: "var(--card-bg)", border: "0.5px solid var(--separator)", borderRadius: 10, padding: "24px 30px 28px" }}>
      <div style={{ display: "flex", alignItems: "baseline", marginBottom: 18 }}>
        <span style={{ font: "var(--font-body-emphasized)", color: "var(--label)", flex: 1 }}>Brief</span>
        <span style={{ font: "var(--font-body)", color: "var(--label-tertiary)" }}>Edit…</span>
      </div>
      <div style={{ font: "400 15px/24px var(--font-system)", color: "var(--label-secondary)", textWrap: "pretty" }}>
        One planning agent exists machine-wide today — agent.PlanSentinel = "plan" and the nat-plan session (internal/agent/tmux.go) — so a workshop session launched on one project reads as the planning agent for every project: the app's AppModel.planningAgent (keyed at the bare sentinel), the workshop pane, the TUI's w, and workshop-launch's refusal of a second all carry it across projects. Scope it per project: the pane tag and status key become project-qualified (e.g. plan:&lt;project page ID&gt;), the session name takes the project ID's tail the way slice sessions take the slice ID's, nat workshop-launch — which already takes --project — refuses a second planning agent for that project only, nat status reports each under its project-qualified key, and the TUI and the app read only the active project's (the app keying planningAgent by active project ID). A pre-upgrade bare "plan" session is still read rather than orphaned — treat it as unscoped legacy that any project may attach. Acceptance: two projects can each run a planning agent at once, switching project tabs never shows another project's workshop session, and Go + macos tests pass.
      </div>
    </div>
  );
}

function U3Meta() {
  const G = ({ label, children }) => (
    <div style={{ marginBottom: 26 }}>
      <div style={{ font: "600 11px/14px var(--font-system)", color: "var(--label-tertiary)", letterSpacing: ".4px", marginBottom: 8 }}>{label}</div>
      {children}
    </div>
  );
  return (
    <div style={{ width: 300, flexShrink: 0, padding: "44px 28px 0", borderLeft: "0.5px solid var(--separator)" }}>
      <G label="STATUS"><span style={{ display: "inline-flex", alignItems: "center", gap: 7, font: "var(--font-body)", color: "var(--label)" }}><span style={{ color: "var(--system-yellow)", fontSize: 8 }}>●</span>In progress</span></G>
      <G label="MILESTONE"><span style={{ font: "var(--font-body)", color: "var(--label)" }}>M38: App fixes</span></G>
      <G label="BRANCH"><span style={{ font: "var(--font-body)", color: "var(--label-secondary)" }}>Assigned on launch</span></G>
      <G label="DEPENDS ON"><span style={{ font: "var(--font-body)", color: "var(--label-secondary)" }}>None</span></G>
    </div>
  );
}

function U3Footer() {
  return (
    <div style={{ display: "flex", justifyContent: "flex-end", padding: "10px 16px", borderTop: "0.5px solid var(--separator)", flexShrink: 0 }}>
      <span style={{ display: "inline-flex", alignItems: "stretch", borderRadius: 6, overflow: "hidden", background: "var(--label-quaternary)" }}>
        <span style={{ display: "inline-flex", alignItems: "center", padding: "0 14px", height: 26, font: "600 13px/16px var(--font-system)", color: "var(--label-tertiary)" }}>Launch Agent</span>
        <span style={{ width: 0.5, background: "var(--separator)" }}></span>
        <span style={{ display: "inline-flex", alignItems: "center", padding: "0 8px" }}><U3Icon name="chevron_down" size={10} weight={700} color="var(--label-tertiary)" style={{ verticalAlign: 0 }} /></span>
      </span>
    </div>
  );
}

function U3Progress() {
  const segs = [[153, 100], [4, 25], [2, 0], [10, 40], [4, 0], [2, 0], [6, 67], [3, 33], [14, 14]];
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 5, padding: "7px 14px", flexShrink: 0, background: "var(--strip-bg)" }}>
      {segs.map(([w, pct], i) => (
        <div key={i} style={{ flex: w, minWidth: 10, height: 8, borderRadius: 4, background: "color-mix(in srgb, var(--label) 12%, transparent)", overflow: "hidden" }}>
          <div style={{ width: `${pct}%`, height: "100%", borderRadius: 4, background: "var(--accent)" }}></div>
        </div>
      ))}
    </div>
  );
}

function U3Shell({ selectedId = "s-scope", tab = "Brief" }) {
  return (
    <div className="nat3">
      <MacWindow width={1440} height={880}>
        <U3Header />
        <div style={{ display: "flex", flex: 1, minHeight: 0 }}>
          <U3Rail selectedId={selectedId} />
          <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column" }}>
            <U3PaneHeader ms="M38: App fixes" title="Scope the planning agent to its project" tab={tab} />
            <div style={{ flex: 1, minHeight: 0, display: "flex" }}>
              <div style={{ flex: 1, minWidth: 0, overflowY: "auto", padding: "0 40px" }}><U3BriefCard /></div>
              <U3Meta />
            </div>
            <U3Footer />
          </div>
        </div>
        <U3Progress />
      </MacWindow>
    </div>
  );
}

Object.assign(window, { U3, U3Shell, U3Rail, U3PaneHeader, U3Tabs, U3BriefCard, U3Meta, U3Progress });

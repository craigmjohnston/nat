const { Icon: VIcon, Badge: VBadge, Button: VBtn, TextField: VTField, SearchField: VSearch, ToolbarButton: VTBBtn, TrafficLights: VTL, Switch: VSwitch } = DS;

function VBar({ pct, tint = "var(--accent)", width = "auto" }) {
  return <div style={{ height: 3, borderRadius: 2, background: "var(--label-quaternary)", width, overflow: "hidden" }}><div style={{ width: `${pct}%`, height: "100%", borderRadius: 2, background: tint }}></div></div>;
}

const V = {
  active: [
    ["s-comments", "Diff comments reach the agent", "Working", "var(--system-orange)", false],
    ["s-mouse", "Board mouse support", "Waiting for input", "var(--system-yellow)", false]
  ],
  review: [
    ["s-syntax", "Syntax highlighting in the diff", "+368 −17", "var(--system-green)"]
  ],
  ms: [
    { num: "4", title: "Diff review", done: 3, total: 5, current: true, slices: [], hidden: 3, elsewhere: 2 },
    { num: "5", title: "Board polish", done: 1, total: 4, slices: [
      ["todo", "s-kanban", "Kanban column view", null],
      ["blocked", "s-wheel", "Wheel scrolling in the Active panel", null]
    ], hidden: 1, elsewhere: 1 },
    { num: "6", title: "Wishlist", done: 0, total: 3, slices: [], collapsed: true }
  ]
};
const VG = { todo: ["circle", "var(--label-tertiary)"], claimed: ["circle_lefthalf_fill", "var(--system-orange)"], done: ["checkmark_circle", "var(--system-green)"], blocked: ["nosign", "var(--label-tertiary)"] };

function V2Header() {
  return (
    <div style={{ display: "flex", alignItems: "stretch", gap: 0, height: 40, background: "color-mix(in srgb, var(--accent) 9%, var(--material-header-bg))", backdropFilter: "var(--material-blur)", flexShrink: 0 }}>
      <div style={{ display: "flex", alignItems: "center", padding: "0 16px" }}><VTL /></div>
      <VProjectTabs />
      <div style={{ display: "flex", alignItems: "center", gap: 12, padding: "0 16px", marginLeft: "auto" }}>
        <span style={{ font: "var(--font-subheadline)", color: "var(--label-tertiary)", fontVariantNumeric: "tabular-nums" }}>21/34 slices</span>
        <VTBBtn icon="plus_rectangle_on_rectangle" title="New Slice" />
        <VTBBtn icon="wand_stars" title="Workshop the Plan" />
      </div>
    </div>
  );
}

const V_PROJECTS = [
  ["nat", "var(--system-orange)", 3, true],
  ["tavern", "var(--system-green)", 1, false],
  ["dotfiles", null, 0, false]
];
function VProjectTabs() {
  return (
    <div style={{ display: "flex", alignItems: "flex-end", gap: 0, paddingTop: 6, alignItems: "flex-end" }}>
      {V_PROJECTS.map(([name, tint, count, on], i) => (
        <React.Fragment key={name}>
          <div title={count ? `${name} — ${count} active session${count > 1 ? "s" : ""}` : name} style={{ display: "flex", alignItems: "center", gap: 7, height: 34, boxSizing: "border-box", padding: "0 22px 6px", minWidth: 130, maxWidth: 220, borderRadius: "10px 10px 0 0", background: on ? "var(--window-bg)" : "transparent", position: "relative" }}>
            {on && <><span style={{ position: "absolute", left: -10, bottom: 0, width: 10, height: 10, background: "radial-gradient(circle at 0 0, transparent 10px, var(--window-bg) 10.5px)" }}></span><span style={{ position: "absolute", right: -10, bottom: 0, width: 10, height: 10, background: "radial-gradient(circle at 100% 0, transparent 10px, var(--window-bg) 10.5px)" }}></span></>}
            {tint && <span className={on ? "" : "ws-pulse"} style={{ color: tint, fontSize: 8 }}>●</span>}
            <span style={{ font: on ? "600 12px/15px var(--font-system)" : "var(--font-subheadline)", color: on ? "var(--label)" : "var(--label-secondary)", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis", flex: 1 }}>{name}</span>
            {count > 0 && <span style={{ font: "var(--font-caption2)", fontVariantNumeric: "tabular-nums", color: on ? "var(--label-secondary)" : "var(--label-tertiary)", background: "var(--label-quaternary)", borderRadius: 8, padding: "0 6px", lineHeight: "14px" }}>{count}</span>}
          </div>
          {!on && !V_PROJECTS[i + 1]?.[3] && <span style={{ width: 1, height: 16, background: "var(--label-quaternary)", alignSelf: "center", marginBottom: 6 }}></span>}
        </React.Fragment>
      ))}
      <div title="New Project Tab" style={{ display: "flex", alignItems: "center", justifyContent: "center", width: 32, height: 32, borderRadius: 7, alignSelf: "center", marginBottom: 6, marginLeft: 6 }}>
        <VIcon name="plus" size={14} color="var(--label-secondary)" style={{ verticalAlign: 0 }} />
      </div>
    </div>
  );
}

function VProgressBorder() {
  const segs = [["Foundations — done 7/7", 7, 100], ["Sessions — done 7/7", 7, 100], ["Review flow — done 7/7", 7, 100], ["Diff review — 3 of 5 done", 5, 60], ["Board polish — 1 of 4 done", 4, 25], ["Wishlist — not started", 3, 0]];
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 4, padding: "6px 20px", flexShrink: 0, background: "var(--window-bg)", borderTop: "0.5px solid var(--separator)" }}>
      {segs.map(([label, w, pct], i) => (
        <div key={i} title={label} style={{ flex: w, height: 7, borderRadius: 3.5, background: "color-mix(in srgb, var(--label) 13%, transparent)", overflow: "hidden" }}>
          <div style={{ width: `${pct}%`, height: "100%", background: pct === 100 ? "var(--system-green)" : "var(--accent)" }}></div>
        </div>
      ))}
    </div>
  );
}

function VChip({ label, current, done }) {
  return (
    <span style={{ width: 18, height: 18, borderRadius: 5, flexShrink: 0, display: "inline-flex", alignItems: "center", justifyContent: "center", font: "var(--font-caption2)", fontVariantNumeric: "tabular-nums", background: done ? "color-mix(in srgb, var(--system-green) 18%, var(--window-bg))" : current ? "color-mix(in srgb, var(--accent) 22%, var(--window-bg))" : "color-mix(in srgb, var(--label) 8%, var(--window-bg))", color: done ? "var(--system-green)" : current ? "var(--accent)" : "var(--label-secondary)", position: "relative", zIndex: 1 }}>
      {done ? <VIcon name="checkmark" size={9} weight={700} color="var(--system-green)" style={{ verticalAlign: 0 }} /> : label}
    </span>
  );
}

function VRing({ pct, label, done, current }) {
  const col = done ? "var(--system-green)" : "var(--accent)";
  return (
    <span style={{ width: 20, height: 20, borderRadius: 10, flexShrink: 0, background: `conic-gradient(${col} ${pct}%, var(--label-quaternary) 0)`, display: "inline-flex", alignItems: "center", justifyContent: "center" }}>
      <span style={{ width: 15, height: 15, borderRadius: 8, background: "var(--control-bg)", display: "inline-flex", alignItems: "center", justifyContent: "center", font: "var(--font-caption2)", color: done ? "var(--system-green)" : current ? "var(--accent)" : "var(--label-secondary)" }}>{done ? <VIcon name="checkmark" size={8} weight={700} color="var(--system-green)" style={{ verticalAlign: 0 }} /> : label}</span>
    </span>
  );
}

function VSliceRow({ s, selectedId }) {
  const [g, c] = VG[s[0]];
  const sel = s[1] === selectedId;
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 8, height: 28, padding: "0 8px", marginLeft: 24, borderRadius: "var(--radius-highlight)", background: sel ? "var(--accent)" : "transparent", color: sel ? "var(--accent-text)" : s[0] === "blocked" ? "var(--label-tertiary)" : "var(--label)" }}>
      <span style={{ width: 13, textAlign: "center", flexShrink: 0, display: "inline-flex", justifyContent: "center" }}><VIcon name={g} size={12} color={sel ? "var(--accent-text)" : c} style={{ verticalAlign: 0 }} /></span>
      <span style={{ flex: 1, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{s[2]}</span>
      {s[3] && <span className={s[3][2] || sel ? "" : "ws-pulse"} style={{ font: "var(--font-subheadline)", color: sel ? "var(--accent-text)" : s[3][1] }}>{s[3][0]}</span>}
    </div>
  );
}

function VRail({ selectedId }) {
  return (
    <div style={{ width: 372, flexShrink: 0, overflowY: "auto", borderRight: "0.5px solid var(--separator)", background: "var(--sidebar-tint, transparent)", padding: "12px 12px 16px" }}>
      <div style={{ display: "flex", alignItems: "center", gap: 6, padding: "0 8px 5px" }}><VIcon name="checkmark_seal" size={11} color="var(--label-tertiary)" style={{ verticalAlign: 0 }} /><span style={{ font: "600 11px/14px var(--font-system)", color: "var(--label-tertiary)" }}>NEEDS REVIEW</span></div>
      {V.review.map(([id, n, meta, tint]) => {
        const sel = id === selectedId;
        return (
          <div key={id} style={{ display: "flex", alignItems: "center", gap: 8, height: 30, padding: "0 8px", borderRadius: "var(--radius-highlight)", background: sel ? "var(--accent)" : "transparent", color: sel ? "var(--accent-text)" : "var(--label)" }}>
            <span style={{ color: sel ? "var(--accent-text)" : tint, fontSize: 9 }}>●</span>
            <span style={{ flex: 1, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{n}</span>
            <span style={{ font: "var(--font-subheadline)", fontVariantNumeric: "tabular-nums", color: sel ? "var(--accent-text)" : "var(--label-tertiary)" }}>{meta}</span>
          </div>
        );
      })}
      <div style={{ font: "600 11px/14px var(--font-system)", color: "var(--label-tertiary)", padding: "10px 8px 5px" }}>ACTIVE</div>
      {V.active.map(([id, n, st, tint, still]) => {
        const sel = id === selectedId;
        return (
          <div key={id} style={{ display: "flex", alignItems: "center", gap: 8, height: 30, padding: "0 8px", borderRadius: "var(--radius-highlight)", background: sel ? "var(--accent)" : "transparent", color: sel ? "var(--accent-text)" : "var(--label)" }}>
            <span className={still || sel ? "" : "ws-pulse"} style={{ color: sel ? "var(--accent-text)" : tint, fontSize: 9 }}>●</span>
            <span style={{ flex: 1, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{n}</span>
            <span style={{ font: "var(--font-subheadline)", color: sel ? "var(--accent-text)" : tint }}>{st}</span>
          </div>
        );
      })}
      <div style={{ borderBottom: "0.5px solid var(--separator)", margin: "10px 0" }}></div>
      <div style={{ border: "0.5px solid var(--separator)", borderRadius: 8, background: "var(--control-bg)", marginBottom: 8 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 9, height: 32, padding: "0 10px" }}>
          <VIcon name="chevron_right" size={9} weight={700} color="var(--label-tertiary)" style={{ verticalAlign: 0 }} />
          <VRing pct={100} done />
          <span style={{ font: "var(--font-subheadline)", color: "var(--label-tertiary)", flex: 1 }}>Done — 3 milestones</span>
          <span style={{ font: "var(--font-subheadline)", color: "var(--label-tertiary)", fontVariantNumeric: "tabular-nums" }}>21/21</span>
        </div>
      </div>
      {V.ms.map((m) => (
        <div key={m.num} style={{ border: "0.5px solid var(--separator)", borderRadius: 8, background: "var(--control-bg)", marginBottom: 8, padding: "2px 4px" }}>
          <div style={{ display: "flex", alignItems: "center", gap: 9, height: 30, padding: "0 6px" }}>
            <VIcon name={m.collapsed ? "chevron_right" : "chevron_down"} size={9} weight={700} color="var(--label-tertiary)" style={{ verticalAlign: 0 }} />
            <VRing pct={m.done / m.total * 100} label={m.num} current={m.current} />
            <span style={{ font: "var(--font-body-emphasized)", flex: 1, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{m.title}</span>
            <span style={{ font: "var(--font-subheadline)", color: "var(--label-secondary)", fontVariantNumeric: "tabular-nums" }}>{m.done}/{m.total}</span>
          </div>
          {!m.collapsed && <div style={{ paddingBottom: 4 }}>
            {m.slices.map((s) => <VSliceRow key={s[1]} s={s} selectedId={selectedId} />)}
            {m.elsewhere > 0 && <div style={{ display: "flex", alignItems: "center", gap: 8, height: 26, marginLeft: 24, padding: "0 8px", color: "var(--label-tertiary)", font: "var(--font-subheadline)" }}>
              <VIcon name="chevron_right" size={8} weight={700} color="var(--label-quaternary)" style={{ verticalAlign: 0 }} />
              <span className="ws-pulse" style={{ color: "var(--system-orange)", fontSize: 8 }}>✻</span>
              <span>{m.elsewhere} in flight</span>
            </div>}
            {m.hidden > 0 && <div style={{ display: "flex", alignItems: "center", gap: 8, height: 26, marginLeft: 24, padding: "0 8px", color: "var(--label-tertiary)", font: "var(--font-subheadline)" }}>
              <VIcon name="chevron_right" size={8} weight={700} color="var(--label-quaternary)" style={{ verticalAlign: 0 }} />
              <VIcon name="checkmark" size={9} weight={700} color="var(--system-green)" style={{ verticalAlign: 0 }} />
              <span>{m.hidden} done</span>
            </div>}
          </div>}
        </div>
      ))}
    </div>
  );
}

function VTabs({ tabs, value }) {
  const cur = tabs.findIndex(([, label]) => label === value);
  return (
    <div style={{ display: "inline-flex", alignItems: "stretch", alignSelf: "stretch", gap: 10 }}>
      {tabs.map(([ic, label], i) => {
        const on = i === cur, past = i < cur, future = i > cur;
        return (
          <React.Fragment key={label}>
            {i > 0 && <span style={{ display: "inline-flex", alignItems: "center" }}><VIcon name="arrow_right" size={12} color="var(--label-quaternary)" style={{ verticalAlign: 0 }} /></span>}
            <span style={{ display: "inline-flex", alignItems: "center", justifyContent: "center", gap: 5, width: 66, position: "relative", opacity: future ? 0.4 : 1 }}>
              <VIcon name={ic} size={12} color={on ? "var(--accent)" : "var(--label-secondary)"} style={{ verticalAlign: 0 }} />
              <span style={{ font: on ? "600 12px/15px var(--font-system)" : "var(--font-subheadline)", color: on ? "var(--label)" : "var(--label-secondary)" }}>{label}</span>
              {past && <VIcon name="checkmark" size={9} weight={700} color="var(--system-green)" style={{ verticalAlign: 0 }} />}
              {on && <span style={{ position: "absolute", left: 6, right: 6, bottom: 0, height: 2, borderRadius: 1, background: "var(--accent)" }}></span>}
            </span>
          </React.Fragment>
        );
      })}
    </div>
  );
}

function VPaneHeader({ title, meta, tabs, value, actions }) {
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 14, height: 46, padding: "0 14px", borderBottom: "0.5px solid var(--separator)", flexShrink: 0 }}>
      <span style={{ font: "var(--font-body-emphasized)", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{title}</span>
      {meta && <span style={{ font: "var(--font-subheadline)", color: "var(--label-tertiary)", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis", minWidth: 0, flexShrink: 2 }}>{meta}</span>}
      <div style={{ flex: 1 }}></div>
      <div style={{ display: "flex", alignItems: "center", gap: 8, flexShrink: 0, whiteSpace: "nowrap" }}>{actions}</div>
      <div style={{ alignSelf: "stretch", flexShrink: 0, display: "inline-flex" }}><VTabs tabs={tabs} value={value} /></div>
    </div>
  );
}

function VShell({ selectedId, children }) {
  return (
    <div className="nat">
      <MacWindow width={1360} height={840}>
        <V2Header />
        <div style={{ display: "flex", flex: 1, minHeight: 0 }}>
          <VRail selectedId={selectedId} />
          <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", background: "var(--control-bg)" }}>{children}</div>
        </div>
        <VProgressBorder />
      </MacWindow>
    </div>
  );
}

const V_TABS_PR = [["doc_text", "Brief", true], ["chevron_left_slash_chevron_right", "Agent", true], ["plusminus", "Diff", true], ["arrow_branch", "PR", true]];
const V_TABS_LIVE = [["doc_text", "Brief", true], ["chevron_left_slash_chevron_right", "Agent", true], ["plusminus", "Diff", true], ["arrow_branch", "PR", false]];
const V_TABS_TODO = [["doc_text", "Brief", true], ["chevron_left_slash_chevron_right", "Agent", false], ["plusminus", "Diff", false], ["arrow_branch", "PR", false]];

Object.assign(window, { V, VShell, VPaneHeader, VTabs, V_TABS_LIVE, V_TABS_TODO, V_TABS_PR, VChip });

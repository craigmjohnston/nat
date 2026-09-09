const DS = window.MacOSNativeDesignSystem_153403;
const { Icon, Badge, Sidebar, SidebarSection, SidebarItem, TrafficLights, Toolbar, ToolbarButton, SearchField, Button } = DS;

function MacWindow({ width, height, children, style }) {
  return <div style={{ width, height, borderRadius: "var(--radius-window)", overflow: "hidden", boxShadow: "var(--shadow-window)", background: "var(--window-bg)", display: "flex", flexDirection: "column", position: "relative", colorScheme: "dark", font: "var(--font-body)", color: "var(--label)", ...style }}>{children}</div>;
}

const PLAN = {
  active: [
    { id: "s-comments", name: "Diff comments reach the agent", state: "Working", tint: "var(--system-orange)", milestone: "Diff review" },
    { id: "s-mouse", name: "Board mouse support", state: "Waiting for input", tint: "var(--system-yellow)", milestone: "Board polish" },
    { id: "s-syntax", name: "Syntax highlighting in the diff", state: "Awaiting review", tint: "var(--system-green)", milestone: "Diff review" }
  ],
  groups: [
    { num: "4", title: "Diff review", count: "3/5", status: "Active", expanded: true, hidden: 3, slices: [
      { id: "s-comments", status: "claimed", name: "Diff comments reach the agent", star: "working", assignee: "craig" },
      { id: "s-syntax", status: "claimed", name: "Syntax highlighting in the diff", review: true, assignee: "craig" }
    ] },
    { num: "5", title: "Board polish", count: "1/4", status: "Active", expanded: true, hidden: 1, slices: [
      { id: "s-mouse", status: "claimed", name: "Board mouse support", star: "waiting", assignee: "craig" },
      { id: "s-kanban", status: "todo", name: "Kanban column view" },
      { id: "s-wheel", status: "todo", name: "Wheel scrolling in the Active panel", blocked: true }
    ] },
    { num: "6", title: "Wishlist", count: "0/3", status: "Todo", expanded: false, slices: [] }
  ],
  doneSummary: "3 milestones · 21/21"
};

const GLYPH = { todo: ["○", "var(--label-tertiary)"], claimed: ["◐", "var(--system-orange)"], done: ["✓", "var(--system-green)"] };

function NatSidebar() {
  return (
    <Sidebar width={208} header={<div style={{ padding: "18px 16px 2px" }}><TrafficLights /></div>}>
      <SidebarSection title="Projects">
        <SidebarItem icon="folder_fill" label="nat" badge={13} selected />
        <SidebarItem icon="folder_fill" label="marmalade-site" badge={4} />
        <SidebarItem icon="folder_fill" label="ledger-import" />
      </SidebarSection>
      <SidebarSection title="" collapsible={false}>
        <SidebarItem icon="plus" label="New Project…" style={{ color: "var(--label-secondary)" }} />
      </SidebarSection>
    </Sidebar>
  );
}

function Star({ kind, selected }) {
  const tint = kind === "waiting" ? "var(--system-yellow)" : "var(--system-orange)";
  return <span title={kind === "waiting" ? "Agent is waiting for input" : "Agent is working"} style={{ color: selected ? "var(--accent-text)" : tint, fontSize: 12 }}>✻</span>;
}

function SliceRow({ s, selected }) {
  const [g, c] = s.blocked ? ["⊘", "var(--label-tertiary)"] : GLYPH[s.status];
  const fg = selected ? "var(--accent-text)" : s.blocked ? "var(--label-tertiary)" : "var(--label)";
  const sub = selected ? "rgba(255,255,255,.72)" : "var(--label-secondary)";
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 7, height: 28, padding: "0 8px 0 30px", borderRadius: "var(--radius-highlight)", background: selected ? "var(--selected-content-bg)" : "transparent", color: fg }}>
      <span style={{ color: selected ? "var(--accent-text)" : c, width: 13, textAlign: "center", fontSize: 12 }}>{g}</span>
      <span style={{ flex: 1, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{s.name}</span>
      {s.star && <Star kind={s.star} selected={selected} />}
      {s.review && <span style={{ font: "var(--font-caption2)", color: selected ? "var(--accent-text)" : "var(--system-green)", border: `1px solid ${selected ? "rgba(255,255,255,.5)" : "color-mix(in srgb, var(--system-green) 55%, transparent)"}`, borderRadius: 9, padding: "1px 6px" }}>↑ review</span>}
      {s.assignee && <span style={{ font: "var(--font-subheadline)", color: sub }}>@{s.assignee}</span>}
    </div>
  );
}

function MilestoneRow({ g, narrow }) {
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 6, height: 28, padding: "0 8px" }}>
      <Icon name={g.expanded ? "chevron_down" : "chevron_right"} size={9} weight={700} color="var(--label-tertiary)" style={{ width: 10, verticalAlign: 0 }} />
      <span style={{ font: "var(--font-subheadline)", color: "var(--label-tertiary)", width: 10, textAlign: "right", fontVariantNumeric: "tabular-nums" }}>{g.num}</span>
      <span style={{ font: "var(--font-body-emphasized)", flex: 1, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{g.title}</span>
      {!narrow && g.hidden > 0 && <span style={{ font: "var(--font-subheadline)", color: "var(--label-tertiary)" }}>{g.hidden} done hidden</span>}
      <span style={{ font: "var(--font-subheadline)", color: "var(--label-secondary)", fontVariantNumeric: "tabular-nums" }}>{g.count}</span>
      <Badge tint={g.status === "Active" ? "var(--system-blue)" : "var(--system-gray)"}>{g.status}</Badge>
    </div>
  );
}

function ActivePanel({ selectedId }) {
  return (
    <div style={{ padding: "10px 8px 2px" }}>
      <div style={{ font: "600 11px/14px var(--font-system)", color: "var(--label-tertiary)", padding: "0 8px 4px" }}>ACTIVE</div>
      {PLAN.active.map((a) => {
        const sel = a.id === selectedId;
        const bg = sel ? "var(--selected-content-bg-unemphasized)" : "transparent";
        return (
          <div key={a.id} style={{ padding: "5px 8px", borderRadius: "var(--radius-highlight)", background: bg }}>
            <div style={{ display: "flex", alignItems: "center", gap: 7 }}>
              <span style={{ color: a.tint, fontSize: 9 }}>●</span>
              <span style={{ color: "var(--label)", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{a.name}</span>
            </div>
            <div style={{ font: "var(--font-subheadline)", paddingLeft: 16, color: "var(--label-secondary)" }}>
              <span style={{ color: a.tint }}>{a.state}</span> · {a.milestone}
            </div>
          </div>
        );
      })}
      <div style={{ borderBottom: "0.5px solid var(--separator)", margin: "10px 0 0" }}></div>
    </div>
  );
}

function PlanColumn({ selectedId, width = 340 }) {
  return (
    <div style={{ width, flexShrink: 0, background: "var(--control-bg)", borderRight: "0.5px solid var(--separator)", overflowY: "auto", display: "flex", flexDirection: "column" }}>
      <ActivePanel selectedId={selectedId} />
      <div style={{ padding: "8px 8px 12px", display: "flex", flexDirection: "column", gap: 1 }}>
        {PLAN.groups.map((g) => (
          <React.Fragment key={g.num}>
            <MilestoneRow g={g} narrow={width < 320} />
            {g.expanded && g.slices.map((s) => <SliceRow key={s.id} s={s} selected={s.id === selectedId} />)}
          </React.Fragment>
        ))}
        <div style={{ display: "flex", alignItems: "center", gap: 6, height: 28, padding: "0 8px", marginTop: 8, borderTop: "0.5px solid var(--separator)" }}>
          <Icon name="chevron_right" size={9} weight={700} color="var(--label-tertiary)" style={{ width: 10, verticalAlign: 0 }} />
          <span style={{ font: "var(--font-body-emphasized)", color: "var(--label-secondary)" }}>Done</span>
          <span style={{ font: "var(--font-subheadline)", color: "var(--label-tertiary)", fontVariantNumeric: "tabular-nums" }}>{PLAN.doneSummary}</span>
        </div>
      </div>
    </div>
  );
}

function MetaRows({ rows }) {
  return (
    <div style={{ background: "var(--control-bg)", borderRadius: "var(--radius-panel)", boxShadow: "var(--shadow-control)" }}>
      {rows.map(([label, value, mono], i) => (
        <div key={label} style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 16, padding: "9px 14px", borderTop: i ? "0.5px solid var(--separator)" : "none" }}>
          <span style={{ color: "var(--label)" }}>{label}</span>
          <span style={{ color: "var(--label-secondary)", font: mono ? "var(--font-code)" : "var(--font-body)", textAlign: "right" }}>{value}</span>
        </div>
      ))}
    </div>
  );
}

Object.assign(window, { DS, MacWindow, PLAN, NatSidebar, PlanColumn, MetaRows, Star });

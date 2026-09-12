const { Icon: NPIcon, TrafficLights: NPTL, ToolbarButton: NPTBBtn } = DS;

const NP_PROJECTS = [
  ["notion-agent-tracker", "var(--accent)", false],
  ["Property market search", "var(--system-red)", false],
  ["Homelab", "var(--system-orange)", false]
];

function NPHeader({ title = "Untitled", untitled = true }) {
  return (
    <div style={{ display: "flex", alignItems: "stretch", height: 44, background: "var(--strip-bg)", flexShrink: 0 }}>
      <div style={{ display: "flex", alignItems: "center", padding: "0 16px" }}><NPTL /></div>
      <div style={{ display: "flex", alignItems: "flex-end" }}>
        {NP_PROJECTS.map(([name, tint], i) => (
          <React.Fragment key={name}>
            <div style={{ display: "flex", alignItems: "center", gap: 8, height: 38, boxSizing: "border-box", padding: "0 14px 4px 18px", minWidth: 150, maxWidth: 250 }}>
              <span style={{ color: tint, fontSize: 8 }}>●</span>
              <span style={{ font: "var(--font-subheadline)", color: "var(--label-secondary)", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis", flex: 1 }}>{name}</span>
            </div>
            <span style={{ width: 1, height: 16, background: "var(--label-quaternary)", alignSelf: "center", marginBottom: 4 }}></span>
          </React.Fragment>
        ))}
        <div style={{ display: "flex", alignItems: "center", gap: 8, height: 38, boxSizing: "border-box", padding: "0 14px 4px 18px", minWidth: 200, maxWidth: 250, borderRadius: "10px 10px 0 0", background: "var(--window-bg)", position: "relative" }}>
          <span style={{ position: "absolute", left: -10, bottom: 0, width: 10, height: 10, background: "radial-gradient(circle at 0 0, transparent 10px, var(--window-bg) 10.5px)" }}></span>
          <span style={{ position: "absolute", right: -10, bottom: 0, width: 10, height: 10, background: "radial-gradient(circle at 100% 0, transparent 10px, var(--window-bg) 10.5px)" }}></span>
          <span style={{ color: untitled ? "var(--label-quaternary)" : "var(--system-teal)", fontSize: 8 }}>●</span>
          <span style={{ font: "600 12px/15px var(--font-system)", color: untitled ? "var(--label-secondary)" : "var(--label)", fontStyle: untitled ? "italic" : "normal", flex: 1 }}>{title}</span>
          <NPIcon name="xmark" size={10} weight={600} color="var(--label-tertiary)" style={{ verticalAlign: 0 }} />
        </div>
        <div title="New project tab" style={{ display: "flex", alignItems: "center", justifyContent: "center", width: 30, height: 30, borderRadius: 7, alignSelf: "center", marginLeft: 10 }}>
          <NPIcon name="plus" size={14} color="var(--label-secondary)" style={{ verticalAlign: 0 }} />
        </div>
      </div>
    </div>
  );
}

function NPRail() {
  return (
    <div style={{ width: 372, flexShrink: 0, borderRight: "0.5px solid var(--separator)", padding: "14px 18px 16px", display: "flex", flexDirection: "column" }}>
      <U3SectionHead icon="bolt" label="ACTIVE" />
      <div style={{ font: "var(--font-subheadline)", color: "var(--label-tertiary)", padding: "1px 8px 0 26px" }}>Nothing running</div>
      <div style={{ borderBottom: "0.5px solid var(--separator)", margin: "12px 0" }}></div>
      <U3SectionHead icon="list_bullet" label="TODO" pad="0 8px 8px 5px" />
      <div style={{ font: "var(--font-subheadline)", color: "var(--label-tertiary)", padding: "0 8px 0 26px", lineHeight: "18px", textWrap: "pretty" }}>Milestones and slices appear here once the project has a plan.</div>
    </div>
  );
}

function NotionMark({ size = 20 }) {
  return (
    <svg width={size} height={size} viewBox="0 0 100 100" style={{ display: "block" }}>
      <path fill="#fff" d="M6.4 4.3 61.4.2c6.8-.6 8.5-.2 12.8 2.9l17.6 12.4c2.9 2.1 3.9 2.7 3.9 5v67.9c0 4.3-1.6 6.8-7 7.2l-63.9 3.9c-4.1.2-6-.4-8.2-3.1L3.7 79.7C1.3 76.5.3 74.1.3 71.3V11.1c0-3.5 1.6-6.4 6.1-6.8z"/>
      <path fill="#111" d="M61.4.2 6.4 4.3C1.9 4.7.3 7.6.3 11.1v60.2c0 2.8 1 5.2 3.4 8.4l12.9 16.7c2.2 2.7 4.1 3.3 8.2 3.1l63.9-3.9c5.4-.4 7-2.9 7-7.2V20.5c0-2.2-.9-2.9-3.5-4.8L74.2 3.1C69.9 0 68.2-.4 61.4.2zM26.2 19.6c-5.2.4-6.4.4-9.4-2L9.3 11.7c-.8-.8-.4-1.7 1.5-1.9l52.9-3.9c4.5-.4 6.8 1.2 8.5 2.5l9.1 6.6c.4.2 1.4 1.3.2 1.3l-54.6 3.3-.7 0zM20.1 88.3V30.8c0-2.5.8-3.7 3.1-3.9l62.7-3.7c2.1-.2 3.1 1.2 3.1 3.7v57.1c0 2.5-.4 4.6-3.9 4.8l-60 3.5c-3.5.2-5-1-5-4zm59.2-54.4c.4 1.7 0 3.5-1.8 3.7l-2.9.6v42.5c-2.5 1.3-4.8 2.1-6.8 2.1-3.1 0-3.9-1-6.2-3.9L42.7 49.2v28.8l6 1.4s0 3.5-4.8 3.5l-13.4.8c-.4-.8 0-2.7 1.4-3.1l3.5-1V41.5l-4.8-.4c-.4-1.7.6-4.3 3.3-4.5l14.4-1 19.7 30.2V39.1l-5-.6c-.4-2.1 1.2-3.7 3.1-3.9l13.2-.7z"/>
    </svg>
  );
}

function NPOpenTile({ glyph, title, sub }) {
  return (
    <div style={{ flex: 1, background: "var(--card-bg)", border: "0.5px solid var(--separator)", borderRadius: 10, padding: "20px 20px 18px", display: "flex", flexDirection: "column", alignItems: "center", textAlign: "center", gap: 4 }}>
      <span style={{ height: 34, display: "inline-flex", alignItems: "center", justifyContent: "center", marginBottom: 6 }}>{glyph}</span>
      <span style={{ font: "var(--font-body-emphasized)", color: "var(--label)" }}>{title}</span>
      <span style={{ font: "var(--font-subheadline)", color: "var(--label-tertiary)", lineHeight: "17px", textWrap: "pretty" }}>{sub}</span>
    </div>
  );
}

function NPStarter() {
  return (
    <div style={{ width: 620, margin: "0 auto", paddingTop: 96 }}>
      <div style={{ font: "600 22px/28px var(--font-system)", color: "var(--label)", marginBottom: 6 }}>Get started</div>
      <div style={{ font: "var(--font-body)", color: "var(--label-secondary)", marginBottom: 28 }}>Start from a description or a plan, or open an existing project.</div>
      <div style={{ background: "var(--card-bg)", border: "0.5px solid var(--separator)", borderRadius: 10, overflow: "hidden" }}>
        <div style={{ padding: "16px 18px 14px" }}>
          <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 10 }}>
            <NPIcon name="wand_stars" size={14} color="var(--accent)" style={{ verticalAlign: 0 }} />
            <span style={{ font: "var(--font-body-emphasized)", color: "var(--label)" }}>Describe a plan</span>
          </div>
          <div style={{ background: "var(--field-bg)", border: "0.5px solid var(--control-border)", borderRadius: 6, padding: "10px 12px", minHeight: 76 }}>
            <span style={{ font: "var(--font-body)", color: "var(--label-tertiary)", lineHeight: "20px" }}>What do you want to do? Sketch the milestones and slices, paste a Notion page or URL, or drop a plan file — the planning agent workshops it into a plan with you.</span>
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 12, marginTop: 12 }}>
            <span style={{ display: "inline-flex", alignItems: "center", gap: 6, font: "var(--font-subheadline)", color: "var(--label-secondary)" }}>
              <NPIcon name="doc" size={12} color="var(--label-tertiary)" style={{ verticalAlign: 0 }} />
              Open plan from filesystem…
            </span>
            <span style={{ flex: 1 }}></span>
            <span style={{ font: "var(--font-subheadline)", color: "var(--label-tertiary)" }}>⌘↩ to start</span>
            <span style={{ display: "inline-flex", alignItems: "center", gap: 6, height: 26, padding: "0 14px", borderRadius: 6, background: "var(--accent)", color: "var(--accent-text)", font: "600 13px/16px var(--font-system)" }}>Workshop the plan</span>
          </div>
        </div>
      </div>
      <div style={{ display: "flex", alignItems: "center", gap: 12, margin: "22px 0" }}>
        <span style={{ flex: 1, borderBottom: "0.5px solid var(--separator)" }}></span>
        <span style={{ font: "var(--font-subheadline)", color: "var(--label-tertiary)" }}>or open an existing project</span>
        <span style={{ flex: 1, borderBottom: "0.5px solid var(--separator)" }}></span>
      </div>
      <div style={{ display: "flex", gap: 14 }}>
        <NPOpenTile glyph={<NotionMark size={26} />} title="From Notion" sub="Pick a project page from your workspace" />
        <NPOpenTile glyph={<NPIcon name="folder" size={24} color="var(--system-blue)" style={{ verticalAlign: 0 }} />} title="From filesystem" sub="Choose a project folder that already has a plan" />
      </div>
    </div>
  );
}

function NPShell() {
  return (
    <div className="nat3">
      <MacWindow width={1440} height={880}>
        <NPHeader />
        <div style={{ display: "flex", flex: 1, minHeight: 0 }}>
          <NPRail />
          <div style={{ flex: 1, minWidth: 0, overflowY: "auto" }}><NPStarter /></div>
        </div>
        <div style={{ height: 22, flexShrink: 0, background: "var(--strip-bg)" }}></div>
      </MacWindow>
    </div>
  );
}

Object.assign(window, { NPShell });

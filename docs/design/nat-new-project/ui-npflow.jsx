const { Icon: NFIcon } = DS;

const NF_TERM_BASE = [
  ["you", "I want nat to manage plans for a Rust rewrite of the importer — milestones for parsing, the ledger model, and CLI parity, with tests gating each one."],
  ["claude", "That splits nicely. Before I draft milestones — should CLI parity mirror the existing flags exactly, or is this the moment to clean up the interface?"],
  ["you", "mirror exactly, cleanup is a later project"],
  ["claude", "Got it. Drafting milestones and slices…"]
];
const NF_TERM_DONE = [["claude", "Proposed 4 milestones · 14 slices. Review them in the sidebar — tell me what to merge, split, or drop and I'll revise."]];

const NF_PROPOSAL = [
  { id: "P1", title: "M1: Parser core", done: 0, total: 4, slices: [
    ["todo", "p-lex", "Tokenize the export format"],
    ["todo", "p-ast", "Parse rows into typed records"],
    ["todo", "p-err", "Surface malformed rows with line context"],
    ["todo", "p-fuzz", "Fuzz the parser against captured exports"]
  ] },
  { id: "P2", title: "M2: Ledger model", done: 0, total: 3, slices: [
    ["todo", "p-model", "Define accounts, postings and balances"],
    ["todo", "p-recon", "Reconcile imported rows against balances"],
    ["todo", "p-round", "Handle rounding and currency minor units"]
  ] },
  { id: "P3", title: "M3: CLI parity", done: 0, total: 4, collapsed: true },
  { id: "P4", title: "M4: Cutover", done: 0, total: 3, collapsed: true }
];

function NFNudgeCard() {
  return (
    <div style={{ marginTop: "auto", background: "var(--card-bg)", border: "0.5px solid var(--separator)", borderRadius: 8, padding: "12px 14px" }}>
      <div style={{ display: "flex", alignItems: "center", gap: 9, marginBottom: 4 }}>
        <NotionMark size={15} />
        <span style={{ font: "var(--font-body-emphasized)", flex: 1 }}>Mirror this plan to Notion?</span>
        <NFIcon name="xmark" size={9} weight={600} color="var(--label-tertiary)" style={{ verticalAlign: 0 }} />
      </div>
      <div style={{ font: "var(--font-subheadline)", color: "var(--label-tertiary)", lineHeight: "17px", marginBottom: 10, textWrap: "pretty" }}>The plan stays local either way — a Notion page keeps it in sync, so anyone on the project can read and edit it.</div>
      <span style={{ display: "inline-flex", alignItems: "center", height: 24, padding: "0 12px", borderRadius: 6, background: "var(--control-face)", border: "0.5px solid var(--control-border)", font: "var(--font-body)" }}>Choose page…</span>
    </div>
  );
}

function NFRail({ stage }) {
  if (stage === "accepted") {
    const ms = NF_PROPOSAL.map((m, i) => (i === 0 ? { ...m, current: true } : m));
    return (
      <div style={{ width: 372, flexShrink: 0, borderRight: "0.5px solid var(--separator)", padding: "14px 18px 16px", display: "flex", flexDirection: "column", minHeight: 0 }}>
        <U3SectionHead icon="bolt" label="ACTIVE" />
        <div style={{ font: "var(--font-subheadline)", color: "var(--label-tertiary)", padding: "1px 8px 0 26px" }}>Nothing running</div>
        <div style={{ borderBottom: "0.5px solid var(--separator)", margin: "12px 0" }}></div>
        <U3SectionHead icon="list_bullet" label="TODO" pad="0 8px 8px 5px" />
        <div style={{ overflowY: "auto", overflowX: "hidden", minHeight: 0 }}>
          {ms.map((m) => <U3MsFolder key={m.id} m={m} selectedId={null} />)}
        </div>
        <NFNudgeCard />
      </div>
    );
  }
  return (
    <div style={{ width: 372, flexShrink: 0, borderRight: "0.5px solid var(--separator)", padding: "14px 18px 16px", display: "flex", flexDirection: "column", minHeight: 0 }}>
      <U3SectionHead icon="bolt" label="ACTIVE" />
      <U3ActiveRow s={["ws", "Workshop the plan", "Working", "var(--system-orange)", "4m", "Planning agent", null]} selected />
      <div style={{ borderBottom: "0.5px solid var(--separator)", margin: "12px 0" }}></div>
      {stage === "proposal" ? (
        <div style={{ flex: 1, minHeight: 0, display: "flex", flexDirection: "column" }}>
          <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "0 8px 8px 5px" }}>
            <span style={{ width: 13, flexShrink: 0, display: "inline-flex", justifyContent: "center" }}><NFIcon name="wand_stars" size={11} color="var(--accent)" style={{ verticalAlign: 0 }} /></span>
            <span style={{ font: "600 11px/14px var(--font-system)", color: "var(--accent)", letterSpacing: ".4px", flex: 1 }}>PROPOSED</span>
            <span style={{ font: "var(--font-subheadline)", color: "var(--label-tertiary)", fontVariantNumeric: "tabular-nums" }}>4 milestones · 14 slices</span>
          </div>
          <div style={{ padding: "0 5px 12px" }}>
            <div style={{ background: "var(--field-bg)", border: "0.5px solid var(--control-border)", borderRadius: 6, padding: "6px 10px", display: "flex", alignItems: "center", gap: 8 }}>
              <span style={{ font: "var(--font-body-emphasized)", color: "var(--label)", flex: 1 }}>rust-importer</span>
              <NFIcon name="pencil" size={11} color="var(--label-tertiary)" style={{ verticalAlign: 0 }} />
            </div>
            <div style={{ font: "var(--font-subheadline)", color: "var(--label-tertiary)", marginTop: 5, padding: "0 2px" }}>Project name — suggested by the planning agent</div>
          </div>
          <div style={{ flex: 1, minHeight: 0, overflowY: "auto", overflowX: "hidden" }}>
            {NF_PROPOSAL.map((m) => <U3MsFolder key={m.id} m={m} selectedId={null} />)}
          </div>
          <div style={{ paddingTop: 14, borderTop: "0.5px solid var(--separator)", marginTop: 12 }}>
            <div style={{ display: "flex", gap: 8 }}>
              <span style={{ flex: 1, display: "inline-flex", alignItems: "center", justifyContent: "center", height: 28, borderRadius: 6, background: "var(--accent)", color: "var(--accent-text)", font: "600 13px/16px var(--font-system)" }}>Accept plan</span>
              <span style={{ flex: 1, display: "inline-flex", alignItems: "center", justifyContent: "center", height: 28, borderRadius: 6, background: "var(--control-face)", border: "0.5px solid var(--control-border)", color: "var(--label)", font: "var(--font-body)" }}>Keep workshopping</span>
            </div>
            <div style={{ font: "var(--font-subheadline)", color: "var(--label-tertiary)", textAlign: "center", marginTop: 10, textWrap: "pretty" }}>Accepting writes the plan to local storage as “rust-importer”.</div>
          </div>
        </div>
      ) : (
        <React.Fragment>
          <U3SectionHead icon="list_bullet" label="TODO" pad="0 8px 8px 5px" />
          <div style={{ font: "var(--font-subheadline)", color: "var(--label-tertiary)", padding: "0 8px 0 26px", lineHeight: "18px", textWrap: "pretty" }}>Milestones and slices land here as the workshop settles on a plan.</div>
        </React.Fragment>
      )}
    </div>
  );
}

function NFTermLine({ who, text }) {
  return (
    <div style={{ display: "flex", gap: 10, marginBottom: 14 }}>
      <span style={{ width: 14, flexShrink: 0, textAlign: "center", color: who === "you" ? "var(--label-tertiary)" : "var(--accent)", font: "var(--font-code)" }}>{who === "you" ? ">" : "⏺"}</span>
      <span style={{ font: "var(--font-code)", fontSize: 12.5, lineHeight: "19px", color: who === "you" ? "var(--label-secondary)" : "var(--label)", textWrap: "pretty" }}>{text}</span>
    </div>
  );
}

function NFTerminal({ stage }) {
  const lines = stage === "proposal" ? [...NF_TERM_BASE, ...NF_TERM_DONE] : NF_TERM_BASE;
  return (
    <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column" }}>
      <div style={{ display: "flex", alignItems: "center", gap: 10, padding: "14px 24px 12px", borderBottom: "0.5px solid var(--separator)", flexShrink: 0 }}>
        <span className="ws-pulse" style={{ color: "var(--system-orange)", fontSize: 9 }}>●</span>
        <span style={{ font: "600 15px/19px var(--font-system)", color: "var(--label)" }}>Workshop the plan</span>
        <span style={{ font: "var(--font-subheadline)", color: "var(--label-tertiary)" }}>Planning agent · claude</span>
        <span style={{ flex: 1 }}></span>
        <span style={{ font: "var(--font-subheadline)", color: "var(--label-tertiary)", fontVariantNumeric: "tabular-nums" }}>4m 12s</span>
      </div>
      <div style={{ flex: 1, minHeight: 0, overflowY: "auto", padding: "22px 28px 0", maxWidth: 860 }}>
        <div style={{ font: "var(--font-code)", fontSize: 12.5, color: "var(--label-tertiary)", marginBottom: 18 }}>✻ Workshopping — describe what the project should do; I'll shape it into milestones and slices.</div>
        {lines.map((l, i) => <NFTermLine key={i} who={l[0]} text={l[1]} />)}
        {stage !== "proposal" && <div style={{ display: "flex", gap: 10 }}><span style={{ width: 14, textAlign: "center", color: "var(--system-orange)", font: "var(--font-code)" }} className="ws-pulse">✻</span><span style={{ font: "var(--font-code)", fontSize: 12.5, color: "var(--label-tertiary)" }}>Drafting… (23s · esc to interrupt)</span></div>}
      </div>
      <div style={{ padding: "16px 28px 18px", flexShrink: 0 }}>
        <div style={{ maxWidth: 860, background: "var(--field-bg)", border: "0.5px solid var(--control-border)", borderRadius: 6, padding: "9px 12px", display: "flex", gap: 10 }}>
          <span style={{ color: "var(--label-tertiary)", font: "var(--font-code)", fontSize: 12.5 }}>&gt;</span>
          <span style={{ font: "var(--font-code)", fontSize: 12.5, color: stage === "proposal" ? "var(--label)" : "var(--label-tertiary)" }}>{stage === "proposal" ? "merge M4 into M3 — cutover is just the last parity slice" : "Reply to the planning agent"}</span>
        </div>
      </div>
    </div>
  );
}

function NFShell({ stage }) {
  return (
    <div className="nat3">
      <MacWindow width={1440} height={880}>
        <NPHeader title={stage === "accepted" ? "rust-importer" : "Untitled"} untitled={stage !== "accepted"} />
        <div style={{ display: "flex", flex: 1, minHeight: 0 }}>
          <NFRail stage={stage} />
          {stage === "accepted" ? (
            <div style={{ flex: 1, minWidth: 0, display: "flex", alignItems: "center", justifyContent: "center" }}>
              <div style={{ textAlign: "center" }}>
                <NFIcon name="checkmark_circle" size={30} color="var(--system-green)" style={{ verticalAlign: 0 }} />
                <div style={{ font: "600 15px/19px var(--font-system)", color: "var(--label)", marginTop: 12 }}>Plan accepted</div>
                <div style={{ font: "var(--font-body)", color: "var(--label-tertiary)", marginTop: 4 }}>4 milestones · 14 slices written locally. Select a slice to begin.</div>
              </div>
            </div>
          ) : (
            <NFTerminal stage={stage} />
          )}
        </div>
        <div style={{ height: 22, flexShrink: 0, background: "var(--strip-bg)" }}></div>
      </MacWindow>
    </div>
  );
}

const NF_PAGES = [
  ["Engineering / Projects", "database", true],
  ["Engineering / Plans", "page", false],
  ["Personal / Side projects", "database", false],
  ["Archive / 2025 plans", "page", false]
];

function NFNotionPicker() {
  return (
    <div className="nat3">
      <div style={{ width: 440, background: "var(--card-bg)", border: "0.5px solid var(--separator)", borderRadius: 10, boxShadow: "var(--shadow-window)", overflow: "hidden", colorScheme: "dark", font: "var(--font-body)", color: "var(--label)" }}>
        <div style={{ padding: "18px 20px 0" }}>
          <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 4 }}>
            <NotionMark size={18} />
            <span style={{ font: "600 15px/19px var(--font-system)" }}>Create Notion project page</span>
          </div>
          <div style={{ font: "var(--font-subheadline)", color: "var(--label-tertiary)", marginBottom: 14 }}>Choose where the project page goes. The plan mirrors there as it changes.</div>
          <div style={{ background: "var(--field-bg)", border: "0.5px solid var(--control-border)", borderRadius: 6, padding: "5px 10px", display: "flex", alignItems: "center", gap: 8, marginBottom: 10 }}>
            <NFIcon name="search" size={12} color="var(--label-tertiary)" style={{ verticalAlign: 0 }} />
            <span style={{ font: "var(--font-body)", color: "var(--label-tertiary)" }}>Search your workspace</span>
          </div>
        </div>
        <div style={{ padding: "0 12px" }}>
          {NF_PAGES.map(([path, kind, sel]) => (
            <div key={path} style={{ display: "flex", alignItems: "center", gap: 10, height: 32, padding: "0 10px", borderRadius: 5, background: sel ? "color-mix(in srgb, var(--accent) 24%, transparent)" : "transparent" }}>
              <NFIcon name={kind === "database" ? "tablecells" : "doc_text"} size={13} color={sel ? "var(--label)" : "var(--label-tertiary)"} style={{ verticalAlign: 0 }} />
              <span style={{ flex: 1, color: "var(--label)" }}>{path}</span>
              {kind === "database" && <span style={{ font: "var(--font-caption2)", color: "var(--label-tertiary)", border: "0.5px solid var(--control-border)", borderRadius: 8, padding: "1px 7px" }}>database</span>}
            </div>
          ))}
        </div>
        <div style={{ display: "flex", justifyContent: "flex-end", gap: 10, padding: "14px 20px 16px", borderTop: "0.5px solid var(--separator)", marginTop: 12 }}>
          <span style={{ display: "inline-flex", alignItems: "center", height: 26, padding: "0 14px", borderRadius: 6, background: "var(--control-face)", border: "0.5px solid var(--control-border)", font: "var(--font-body)", color: "var(--label)" }}>Cancel</span>
          <span style={{ display: "inline-flex", alignItems: "center", height: 26, padding: "0 14px", borderRadius: 6, background: "var(--accent)", color: "var(--accent-text)", font: "600 13px/16px var(--font-system)" }}>Create page</span>
        </div>
      </div>
    </div>
  );
}

Object.assign(window, { NFShell, NFNotionPicker });

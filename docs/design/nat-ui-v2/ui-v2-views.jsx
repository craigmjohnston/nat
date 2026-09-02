const { Icon: WIcon, Badge: WBadge, Button: WBtn, TextField: WTField, Switch: WSwitch } = DS;

function AgentView() {
  const chip = (t) => <span style={{ display: "inline-flex", alignItems: "center", gap: 4, font: "var(--font-code)", fontSize: 11, background: "var(--control-face)", border: "0.5px solid var(--control-border)", borderRadius: 5, padding: "0 6px", color: "var(--accent)", whiteSpace: "nowrap", verticalAlign: "baseline" }}>{t}</span>;
  return (
    <VShell selectedId="s-comments">
      <VPaneHeader tabs={V_TABS_LIVE} value="Agent" title="Diff comments reach the agent" meta="nat/diff-comments" />
      <div style={{ flex: 1, minHeight: 0, overflowY: "auto", padding: "16px 22px", display: "flex", flexDirection: "column", gap: 18, maxWidth: 680 }}>
        <div style={{ flexShrink: 0 }}>
          <div style={{ font: "600 11px/14px var(--font-system)", color: "var(--label-tertiary)", marginBottom: 5 }}>YOU · 14:47</div>
          <div style={{ background: "color-mix(in srgb, var(--accent) 12%, transparent)", border: "0.5px solid color-mix(in srgb, var(--accent) 25%, transparent)", borderRadius: 10, padding: "9px 13px", maxWidth: 520, lineHeight: "19px" }}>Comments should reach the agent as one prompt block per file, with real line numbers.</div>
        </div>
        <div style={{ flexShrink: 0 }}>
          <div style={{ font: "600 11px/14px var(--font-system)", color: "var(--label-tertiary)", marginBottom: 5 }}><span className="ws-pulse" style={{ color: "var(--system-orange)" }}>✻</span> AGENT · WORKING</div>
          <div style={{ display: "flex", flexDirection: "column", gap: 11 }}>
            <div style={{ color: "var(--label-secondary)", lineHeight: "19px" }}>Comments now serialise into the hand-back prompt, one block per file with the line numbers the gutter shows — {chip("diffcomment.go")} collects them per hunk, {chip("diffcomment_test.go")} round-trips a two-file set. Pushing the branch so the board can read the diff.</div>
            <div style={{ border: "0.5px solid var(--control-border)", borderRadius: 10, overflow: "hidden" }}>
              <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "7px 12px", background: "var(--row-alt-bg)" }}>
                <WIcon name="chevron_down" size={9} weight={700} color="var(--label-tertiary)" style={{ verticalAlign: 0 }} />
                <span style={{ font: "var(--font-body-emphasized)" }}>Working for 12 minutes</span>
                <span style={{ font: "var(--font-subheadline)", color: "var(--label-tertiary)" }}>4 tool calls</span>
                <div style={{ flex: 1 }}></div>
              </div>
              {[["pencil", "Edit diffcomment.go", "+28 −6", "checkmark", "var(--system-green)"], ["pencil", "Edit diffcomment_test.go", "+64", "checkmark", "var(--system-green)"], ["chevron_left_slash_chevron_right", "go test ./internal/tui/…", "passed · 214 tests", "checkmark", "var(--system-green)"], ["arrow_up", "git push -u origin nat/diff-comments", "waiting", "hourglass", "var(--system-yellow)"]].map(([ic, l, r, st, c]) => (
                <div key={l} style={{ display: "flex", alignItems: "center", gap: 9, height: 28, padding: "0 12px", borderTop: "0.5px solid var(--separator)" }}>
                  <WIcon name={ic} size={12} color="var(--label-tertiary)" style={{ verticalAlign: 0, width: 14 }} />
                  <span style={{ font: "var(--font-code)", flex: 1, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{l}</span>
                  <span style={{ font: "var(--font-caption2)", color: "var(--label-tertiary)" }}>{r}</span>
                  <WIcon name={st} size={11} color={c} style={{ verticalAlign: 0 }} />
                </div>
              ))}
            </div>
            <div style={{ background: "var(--row-alt-bg)", border: "0.5px solid var(--control-border)", borderRadius: "var(--radius-panel)", padding: "11px 14px", display: "flex", flexDirection: "column", gap: 8 }}>
              <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                <WIcon name="exclamationmark_triangle" size={13} color="var(--system-yellow)" style={{ verticalAlign: 0 }} />
                <span style={{ font: "var(--font-body-emphasized)" }}>Wants to run a command</span>
                <div style={{ flex: 1 }}></div>
                <span style={{ font: "var(--font-subheadline)", color: "var(--label-tertiary)" }}>5 s ago</span>
              </div>
              <div style={{ font: "var(--font-code)", background: "var(--field-bg)", border: "0.5px solid var(--control-border)", borderRadius: "var(--radius-control)", padding: "6px 10px" }}>git push -u origin nat/diff-comments</div>
              <div style={{ display: "flex", justifyContent: "flex-end", gap: 8 }}>
                <WBtn size="small">Don't Allow</WBtn>
                <WBtn size="small" variant="prominent">Allow</WBtn>
              </div>
            </div>
          </div>
        </div>
      </div>
      <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "5px 18px", borderTop: "0.5px solid var(--separator)", background: "var(--row-alt-bg)", flexShrink: 0 }}>
        <span className="ws-pulse" style={{ color: "var(--system-orange)", fontSize: 10 }}>✻</span>
        <span style={{ font: "var(--font-subheadline)" }}>Working — pushing <span style={{ font: "var(--font-code)", fontSize: 11 }}>nat/diff-comments</span></span>
        <div style={{ flex: 1 }}></div>
        <span style={{ font: "var(--font-caption2)", color: "var(--label-tertiary)", fontVariantNumeric: "tabular-nums" }}>12m · ↑12.4k tokens</span>
      </div>
      <div style={{ margin: "10px 14px 14px", border: "0.5px solid var(--control-border)", borderRadius: 10, background: "var(--field-bg)", boxShadow: "var(--shadow-control)", flexShrink: 0 }}>
        <div style={{ padding: "10px 13px 2px", minHeight: 40, color: "var(--label-tertiary)" }}>Message the agent…</div>
        <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "7px 10px" }}>
          <span style={{ font: "var(--font-caption2)", color: "var(--label-tertiary)", background: "var(--label-quaternary)", borderRadius: 5, padding: "2px 7px" }}>opus · high</span>
          <span style={{ font: "var(--font-caption2)", color: "var(--label-tertiary)" }}>⌘↩ to send</span>
          <div style={{ flex: 1 }}></div>
          <WBtn size="small" variant="borderless">Terminal…</WBtn>
          <WBtn size="small" variant="borderless">Interrupt</WBtn>
          <WBtn size="small" variant="prominent">Send</WBtn>
        </div>
      </div>
    </VShell>
  );
}

function SplitLaunch() {
  return (
    <span style={{ display: "inline-flex", borderRadius: "var(--radius-control)", overflow: "hidden", boxShadow: "var(--shadow-control)" }}>
      <span style={{ background: "var(--accent)", color: "var(--accent-text)", font: "var(--font-subheadline)", fontWeight: 600, padding: "0 10px", height: 22, display: "inline-flex", alignItems: "center" }}>Launch Agent</span>
      <span style={{ background: "var(--accent)", borderLeft: "1px solid color-mix(in srgb, var(--accent-text) 25%, transparent)", width: 20, display: "inline-flex", alignItems: "center", justifyContent: "center" }}><WIcon name="chevron_down" size={9} weight={700} color="var(--accent-text)" style={{ verticalAlign: 0 }} /></span>
    </span>
  );
}

function LaunchPopover() {
  return (
    <div style={{ position: "absolute", right: 14, bottom: 52, width: 296, background: "var(--control-bg)", border: "0.5px solid var(--control-border)", borderRadius: "var(--radius-panel)", boxShadow: "var(--shadow-popover)", zIndex: 5 }}>
      {[["Model", ["Default", "sonnet", "opus"], "opus"], ["Effort", ["low", "med", "high", "max"], "high"]].map(([label, opts, val], i) => (
        <div key={label} style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "10px 13px", borderTop: i ? "0.5px solid var(--separator)" : "none" }}>
          <span style={{ font: "var(--font-subheadline)" }}>{label}</span>
          {label === "Model" ? (
            <span style={{ display: "inline-flex", alignItems: "center", gap: 7, height: 24, padding: "0 5px 0 10px", borderRadius: "var(--radius-control)", border: "0.5px solid var(--control-border)", background: "var(--control-face)", boxShadow: "var(--shadow-control)", font: "var(--font-subheadline)" }}>
              {val}
              <span style={{ display: "inline-flex", alignItems: "center", justifyContent: "center", width: 16, height: 16, borderRadius: 4, background: "var(--accent)" }}><WIcon name="chevron_up_chevron_down" size={9} weight={700} color="var(--accent-text)" style={{ verticalAlign: 0 }} /></span>
            </span>
          ) : (
            <div style={{ display: "inline-flex", background: "var(--label-quaternary)", borderRadius: 7, padding: 1, height: 24, boxSizing: "border-box" }}>
              {opts.map((o) => <span key={o} style={{ display: "inline-flex", alignItems: "center", padding: "0 10px", borderRadius: 6, font: "var(--font-subheadline)", color: o === val ? "var(--label)" : "var(--label-secondary)", background: o === val ? "var(--control-face)" : "transparent", boxShadow: o === val ? "var(--shadow-control)" : "none" }}>{o}</span>)}
            </div>
          )}
        </div>
      ))}
      <div style={{ padding: "8px 13px 10px", borderTop: "0.5px solid var(--separator)", font: "var(--font-caption2)", color: "var(--label-tertiary)" }}>Runs detached in tmux — closing nat won't stop it.</div>
    </div>
  );
}

function BriefView() {
  return (
    <VShell selectedId="s-kanban">
      <div style={{ position: "relative", display: "flex", flexDirection: "column", flex: 1, minHeight: 0 }}>
        <VPaneHeader tabs={V_TABS_TODO} value="Brief" title="Kanban column view" meta="○ Todo · Board polish" />
        <LaunchPopover />
        <div style={{ flex: 1, minHeight: 0, overflowY: "auto", padding: "18px 22px", display: "flex", flexDirection: "column", gap: 16, maxWidth: 640 }}>
          <div>
            <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
              <span style={{ font: "600 11px/14px var(--font-system)", color: "var(--label-tertiary)", flex: 1 }}>BRIEF — BECOMES THE AGENT'S PROMPT</span>
              <WBtn size="small" variant="borderless">Edit…</WBtn>
            </div>
            <div style={{ marginTop: 6, color: "var(--label)", lineHeight: "19px" }}>Draw the plan as one column per milestone, slices as cards. Keyboard model stays the same; the view is a presentation of the same rows.</div>
            <div style={{ marginTop: 10, font: "var(--font-subheadline)", color: "var(--label-tertiary)" }}>Nothing depends on this slice · branch <span style={{ fontFamily: "var(--font-mono)" }}>nat/kanban-columns</span> cut from main on launch</div>
          </div>
        </div>
        <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "8px 14px", borderTop: "0.5px solid var(--separator)", flexShrink: 0 }}>
          <WBtn size="small" variant="borderless">Edit Brief…</WBtn>
          <div style={{ flex: 1 }}></div>
          <SplitLaunch />
        </div>
      </div>
    </VShell>
  );
}

const W_FILES = [
  ["internal/tui/diffsyntax.go", "+142", "", true, false, "A"],
  ["internal/tui/diffsyntax_test.go", "+168", "", false, false, "A"],
  ["internal/tui/diff.go", "+31", "−12", false, true, "M"],
  ["internal/tui/diffbox.go", "+9", "−3", false, false, "M"],
  ["internal/tui/styles.go", "+18", "−2", true, false, "M"],
  ["go.mod", "+2", "", true, false, "M"]
];
function synGo(text) {
  const out = []; let i = 0, m;
  const re = /([A-Za-z_]\w*)(?=\()/g;
  while ((m = re.exec(text))) {
    if (m.index > i) out.push(text.slice(i, m.index));
    out.push(<span key={m.index} style={{ color: "var(--system-teal)" }}>{m[1]}</span>);
    i = m.index + m[0].length;
  }
  if (i < text.length) out.push(text.slice(i));
  return out;
}
const W_DIFF = [
  [0, 0, "@@ −82,12 +82,20 @@ func (d Diff) View() string", "hunk"],
  [88, 88, "func (d Diff) runStyle(kind runKind, base lipgloss.Style) lipgloss.Style {", ""],
  [89, 0, "\treturn base", "del"],
  [0, 89, "\tswitch kind {", "add"],
  [0, 90, "\tcase runKeyword:", "add"],
  [0, 91, "\t\treturn d.styles.DiffKeyword.Background(base.GetBackground())", "add"],
  [0, 92, "\tcase runString:", "add"],
  [0, 93, "\t\treturn d.styles.DiffString.Background(base.GetBackground())", "add", true],
  [0, 94, "\tcase runComment:", "add"],
  [0, 95, "\t\treturn d.styles.DiffCommentRun.Background(base.GetBackground())", "add"],
  [0, 96, "\t}", "add"],
  [0, 97, "\treturn base", "add"],
  [90, 98, "}", ""]
];

function DiffView() {
  return (
    <VShell selectedId="s-syntax">
      <VPaneHeader tabs={V_TABS_LIVE} value="Diff" title="Syntax highlighting in the diff" meta="+370 −17 · 3 of 6 viewed" />
      <div style={{ display: "flex", flex: 1, minHeight: 0 }}>
        <div style={{ width: 232, flexShrink: 0, order: 2, borderLeft: "0.5px solid var(--separator)", overflowY: "auto", padding: "10px 8px" }}>
          <div style={{ display: "flex", alignItems: "center", gap: 6, margin: "0 4px 8px", height: 22, padding: "0 6px 0 9px", borderRadius: "var(--radius-control)", border: "0.5px solid var(--control-border)", background: "var(--control-face)", boxShadow: "var(--shadow-control)", font: "var(--font-subheadline)" }}>
            <span style={{ flex: 1 }}>All commits</span>
            <span style={{ font: "var(--font-caption2)", color: "var(--label-tertiary)", fontVariantNumeric: "tabular-nums" }}>3</span>
            <span style={{ display: "inline-flex", alignItems: "center", justifyContent: "center", width: 14, height: 14, borderRadius: 4, background: "var(--accent)" }}><WIcon name="chevron_up_chevron_down" size={8} weight={700} color="var(--accent-text)" style={{ verticalAlign: 0 }} /></span>
          </div>
          {W_FILES.map(([path, add, del, viewed, sel, st]) => {
            const dir = path.includes("/") ? path.slice(0, path.lastIndexOf("/") + 1) : "";
            return (
              <div key={path} style={{ display: "flex", alignItems: "center", gap: 5, height: 28, padding: "0 8px", borderRadius: "var(--radius-highlight)", background: sel ? "var(--accent)" : "transparent", color: sel ? "var(--accent-text)" : "var(--label)" }}>
                {viewed ? <WIcon name="checkmark" size={9} weight={700} color={sel ? "var(--accent-text)" : "var(--system-green)"} style={{ width: 11, verticalAlign: 0 }} /> : <span style={{ width: 11 }}></span>}
                <span style={{ flex: 1, minWidth: 0, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis", direction: "rtl", textAlign: "left", font: "var(--font-subheadline)", opacity: viewed && !sel ? 0.55 : 1 }}>{dir}{path.slice(dir.length)}</span>
                {sel && <span style={{ display: "inline-flex", alignItems: "center", gap: 3, font: "var(--font-caption2)", color: "var(--accent-text)" }}><WIcon name="text_bubble" size={10} color="var(--accent-text)" style={{ verticalAlign: 0 }} />1</span>}
                <span style={{ width: 13, height: 13, borderRadius: 3.5, display: "inline-flex", alignItems: "center", justifyContent: "center", font: "600 8px/1 var(--font-system)", background: sel ? "color-mix(in srgb, var(--accent-text) 25%, transparent)" : `color-mix(in srgb, ${st === "A" ? "var(--system-green)" : "var(--system-orange)"} 18%, transparent)`, color: sel ? "var(--accent-text)" : st === "A" ? "var(--system-green)" : "var(--system-orange)" }}>{st}</span>
                <span style={{ font: "var(--font-caption2)", fontVariantNumeric: "tabular-nums", color: sel ? "var(--accent-text)" : "var(--system-green)" }}>{add}</span>
                {del && <span style={{ font: "var(--font-caption2)", fontVariantNumeric: "tabular-nums", color: sel ? "var(--accent-text)" : "var(--system-red)" }}>{del}</span>}
              </div>
            );
          })}
        </div>
        <div style={{ flex: 1, minWidth: 0, overflowY: "auto", padding: "14px 16px" }}>
          <div style={{ border: "0.5px solid var(--control-border)", borderRadius: "var(--radius-panel)", overflow: "hidden" }}>
            <div style={{ display: "flex", alignItems: "center", gap: 10, height: 32, padding: "0 12px", background: "var(--row-alt-bg)", borderBottom: "0.5px solid var(--separator)" }}>
              <WIcon name="chevron_down" size={9} weight={700} color="var(--label-tertiary)" style={{ verticalAlign: 0 }} />
              <span style={{ font: "var(--font-code)" }}>internal/tui/diff.go</span>
              <div style={{ flex: 1 }}></div>
              <span style={{ font: "var(--font-caption2)", color: "var(--system-green)" }}>+31</span>
              <span style={{ font: "var(--font-caption2)", color: "var(--system-red)" }}>−12</span>
              <WBtn size="small" variant="borderless">Viewed</WBtn>
            </div>
            <div style={{ font: "var(--font-code)" }}>
              {W_DIFF.map(([was, now, t, k, hasComment], i) => (
                k === "hunk" ? (
                  <div key={i} style={{ display: "flex", alignItems: "stretch", minHeight: 24 }}>
                    <div style={{ display: "flex", alignItems: "center", justifyContent: "center", flexShrink: 0, width: 84, marginRight: 12, borderRight: "0.5px solid var(--separator)", background: "color-mix(in srgb, var(--accent) 10%, transparent)", color: "var(--accent)" }}>···</div>
                    <span style={{ alignSelf: "center", color: "var(--label-tertiary)" }}>{t}</span>
                  </div>
                ) : (
                <React.Fragment key={i}>
                  <div style={{ display: "flex", alignItems: "stretch", minHeight: 19, background: k === "add" ? "color-mix(in srgb, var(--system-green) 20%, transparent)" : k === "del" ? "color-mix(in srgb, var(--system-red) 20%, transparent)" : "transparent" }}>
                    <div style={{ display: "flex", alignItems: "center", flexShrink: 0, padding: "0 10px 0 12px", marginRight: 12, borderRight: "0.5px solid var(--separator)", background: k === "add" ? "color-mix(in srgb, var(--system-green) 32%, transparent)" : k === "del" ? "color-mix(in srgb, var(--system-red) 32%, transparent)" : "var(--row-alt-bg)" }}>
                      <span style={{ width: 28, textAlign: "right", color: "var(--label-tertiary)", fontVariantNumeric: "tabular-nums" }}>{was || ""}</span>
                      <span style={{ width: 28, textAlign: "right", color: "var(--label-tertiary)", fontVariantNumeric: "tabular-nums", marginLeft: 6 }}>{now || ""}</span>
                    </div>
                    <span style={{ width: 10, flexShrink: 0, alignSelf: "center", color: k === "add" ? "var(--system-green)" : k === "del" ? "var(--system-red)" : "transparent" }}>{k === "add" ? "+" : k === "del" ? "−" : " "}</span>
                    <span style={{ whiteSpace: "pre", alignSelf: "center", color: k === "del" ? "var(--label-secondary)" : "var(--label)" }}>{k === "del" ? t.replace(/\t/g, "    ") : synGo(t.replace(/\t/g, "    "))}</span>
                  </div>
                  {hasComment && (
                    <div style={{ borderTop: "0.5px solid var(--separator)", borderBottom: "0.5px solid var(--separator)", background: "var(--row-alt-bg)", padding: "10px 12px 10px 80px", font: "var(--font-body)" }}>
                      <div style={{ maxWidth: 560, background: "var(--control-bg)", border: "0.5px solid var(--control-border)", borderRadius: 8, overflow: "hidden" }}>
                        <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "7px 12px", background: "var(--row-alt-bg)", borderBottom: "0.5px solid var(--separator)" }}>
                          <span style={{ width: 20, height: 20, borderRadius: 10, background: "color-mix(in srgb, var(--accent) 30%, transparent)", color: "var(--accent)", display: "inline-flex", alignItems: "center", justifyContent: "center", font: "600 9px/1 var(--font-system)" }}>CJ</span>
                          <span style={{ font: "var(--font-body-emphasized)" }}>craig</span>
                          <WBadge tint="var(--system-yellow)">Pending</WBadge>
                          <div style={{ flex: 1 }}></div>
                          <WIcon name="pencil" size={12} color="var(--label-tertiary)" style={{ verticalAlign: 0 }} />
                          <WIcon name="trash" size={12} color="var(--label-tertiary)" style={{ verticalAlign: 0 }} />
                        </div>
                        <div style={{ padding: "9px 12px" }}>Use the wash helper here — Background() drops the selected row's fill.</div>
                      </div>
                      <div style={{ maxWidth: 560, marginTop: 8, font: "var(--font-subheadline)", color: "var(--label-tertiary)" }}>Pending comments go to the agent together — <span style={{ color: "var(--accent)" }}>Send 2 Comments</span> starts a follow-up session.</div>
                    </div>
                  )}
                </React.Fragment>
                )
              ))}
            </div>
          </div>
          <div style={{ marginTop: 10, border: "0.5px solid var(--control-border)", borderRadius: "var(--radius-panel)", display: "flex", alignItems: "center", gap: 10, height: 32, padding: "0 12px", background: "var(--row-alt-bg)" }}>
            <WIcon name="chevron_right" size={9} weight={700} color="var(--label-tertiary)" style={{ verticalAlign: 0 }} />
            <span style={{ font: "var(--font-code)" }}>internal/tui/diffbox.go</span>
            <div style={{ flex: 1 }}></div>
            <span style={{ font: "var(--font-caption2)", color: "var(--system-green)" }}>+9</span>
            <span style={{ font: "var(--font-caption2)", color: "var(--system-red)" }}>−3</span>
            <WBtn size="small" variant="borderless">Viewed</WBtn>
          </div>
        </div>
      </div>
      <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "8px 14px", borderTop: "0.5px solid var(--separator)", flexShrink: 0 }}>
        <span style={{ font: "var(--font-subheadline)", color: "var(--label-tertiary)" }}>1 pending comment</span>
        <div style={{ flex: 1 }}></div>
        <WBtn>Send 2 Comments</WBtn>
        <WBtn variant="prominent">Approve & Open PR…</WBtn>
      </div>
    </VShell>
  );
}

function AgentTerminalView() {
  const dim = "rgba(233,233,239,.45)", fg = "#e9e9ef", or = "#f2a87e", gr = "#a8dea1", lv = "#b1a8f2";
  const L = (parts, i) => <div key={i} style={{ whiteSpace: "pre-wrap", minHeight: 18 }}>{parts.map(([t, c, b], j) => <span key={j} style={{ color: c || fg, fontWeight: b ? 600 : 400 }}>{t}</span>)}</div>;
  const lines = [
    [["╭─ ", dim], ["Claude Code", lv, 1], [" v2.1.4 ─ opus · high ─ nat/diff-comments ─╮", dim]],
    [["", fg]],
    [["> ", lv, 1], ["Comments should reach the agent as one prompt block per file, with real line numbers.", fg]],
    [["", fg]],
    [["● ", gr], ["I'll serialise pending comments into the hand-back prompt, one block per file.", fg]],
    [["", fg]],
    [["● ", gr], ["Update(internal/tui/diffcomment.go)", fg]],
    [["  ⎿  ", dim], ["Updated internal/tui/diffcomment.go with 28 additions and 6 removals", dim]],
    [["       ", dim], ["88   func (c *Collector) Blocks() []Block {", dim]],
    [["       ", dim], ["89 ", dim], ["+      sort.SliceStable(c.pending, func(i, j int) bool {", gr]],
    [["       ", dim], ["90 ", dim], ["+          return c.pending[i].Line < c.pending[j].Line", gr]],
    [["       ", dim], ["91 ", dim], ["+      })", gr]],
    [["", fg]],
    [["● ", gr], ["Bash(go test ./internal/tui/…)", fg]],
    [["  ⎿  ", dim], ["ok   nat/internal/tui   1.24s   214 tests", dim]],
    [["", fg]],
    [["● ", or], ["Bash(git push -u origin nat/diff-comments)", fg]],
    [["  ⎿  ", dim], ["Waiting for approval…", or]],
    [["", fg]],
    [["✻ ", or], ["Pondering… ", fg], ["(12m · ↑12.4k tokens · esc to interrupt)", dim]]
  ];
  return (
    <VShell selectedId="s-comments">
      <VPaneHeader tabs={V_TABS_LIVE} value="Agent" title="Diff comments reach the agent" meta="nat/diff-comments" />
      <div style={{ flex: 1, minHeight: 0, overflowY: "auto", background: "#121216", padding: "14px 18px", font: "var(--font-code)", fontSize: 12, lineHeight: "18px" }}>
        {lines.map(L)}
      </div>
      <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "8px 14px", borderTop: "0.5px solid var(--separator)", background: "#121216", flexShrink: 0, font: "var(--font-code)", fontSize: 12 }}>
        <span style={{ color: lv, fontWeight: 600 }}>&gt;</span>
        <span style={{ color: dim }}>Type a message or slash command…</span>
        <span style={{ width: 7, height: 15, background: fg, display: "inline-block" }} className="ws-pulse"></span>
        <div style={{ flex: 1 }}></div>
        <span style={{ color: dim }}>⏵⏵ accept edits on</span>
      </div>
      <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "7px 14px", borderTop: "0.5px solid var(--separator)", flexShrink: 0 }}>
        <div style={{ flex: 1 }}></div>
        <WBtn size="small" variant="borderless">Open in Terminal…</WBtn>
        <WBtn size="small" variant="borderless">Interrupt</WBtn>
      </div>
    </VShell>
  );
}

function PRComposer({ placeholder, compact }) {
  return (
    <div style={{ border: "0.5px solid var(--control-border)", borderRadius: 8, background: "var(--field-bg)", boxShadow: "var(--shadow-control)" }}>
      <div style={{ padding: compact ? "7px 11px 2px" : "9px 12px 2px", minHeight: compact ? 20 : 34, color: "var(--label-tertiary)", lineHeight: "18px" }}>{placeholder}</div>
      <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "5px 8px 7px" }}>
        <WIcon name="textformat" size={12} color="var(--label-tertiary)" style={{ verticalAlign: 0 }} />
        <WIcon name="paperclip" size={12} color="var(--label-tertiary)" style={{ verticalAlign: 0 }} />
        <div style={{ flex: 1 }}></div>
        <span title="Send" style={{ display: "inline-flex", alignItems: "center", justifyContent: "center", width: 24, height: 22, borderRadius: "var(--radius-control)", background: "var(--accent)", boxShadow: "var(--shadow-control)" }}><WIcon name="paperplane_fill" size={11} color="var(--accent-text)" style={{ verticalAlign: 0 }} /></span>
      </div>
    </div>
  );
}
function PRComment({ av, who, t, body, nested }) {
  return (
    <div style={{ display: "flex", gap: 10, marginLeft: nested ? 28 : 0 }}>
      <span style={{ width: 22, height: 22, borderRadius: 11, flexShrink: 0, background: "color-mix(in srgb, var(--accent) 30%, transparent)", color: "var(--accent)", display: "inline-flex", alignItems: "center", justifyContent: "center", font: "600 9px/1 var(--font-system)", marginTop: 2 }}>{av}</span>
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
          <span style={{ font: "var(--font-body-emphasized)" }}>{who}</span>
          <span style={{ font: "var(--font-caption2)", color: "var(--label-tertiary)" }}>{t}</span>
          <div style={{ flex: 1 }}></div>
          <span title="Quote reply" style={{ display: "inline-flex" }}><WIcon name="text_quote" size={12} color="var(--label-tertiary)" style={{ verticalAlign: 0 }} /></span>
          {who === "craig" && <WIcon name="pencil" size={12} color="var(--label-tertiary)" style={{ verticalAlign: 0 }} />}
        </div>
        <div style={{ marginTop: 3, lineHeight: "19px", color: "var(--label-secondary)" }}>{body}</div>
      </div>
    </div>
  );
}

function PRView() {
  return (
    <VShell selectedId="s-syntax">
      <VPaneHeader tabs={V_TABS_PR} value="PR" title="Syntax highlighting in the diff" meta="nat/diff-syntax → main" />
      <div style={{ display: "flex", flex: 1, minHeight: 0 }}>
        <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column" }}>
          <div style={{ flex: 1, minHeight: 0, overflowY: "auto", padding: "18px 22px", display: "flex", flexDirection: "column", gap: 16 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 10, flexShrink: 0 }}>
            <span style={{ display: "inline-flex", alignItems: "center", gap: 5, height: 22, padding: "0 10px", borderRadius: 11, background: "color-mix(in srgb, var(--system-green) 18%, transparent)", color: "var(--system-green)", font: "var(--font-subheadline)", fontWeight: 600, flexShrink: 0 }}><WIcon name="arrow_branch" size={11} color="var(--system-green)" style={{ verticalAlign: 0 }} />Open</span>
            <span style={{ font: "var(--font-headline)", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>tui: syntax highlighting in the diff view</span>
            <span style={{ font: "var(--font-subheadline)", color: "var(--label-tertiary)" }}>#482</span>
            <WIcon name="pencil" size={12} color="var(--label-tertiary)" style={{ verticalAlign: 0 }} />
            <div style={{ flex: 1 }}></div>
          </div>
          <div style={{ flexShrink: 0 }}>
            <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 6 }}>
              <span style={{ font: "600 11px/14px var(--font-system)", color: "var(--label-tertiary)" }}>DESCRIPTION</span>
              <WBtn size="small" variant="borderless">Edit…</WBtn>
            </div>
            <div style={{ color: "var(--label-secondary)", lineHeight: "19px" }}>Tokenises diff lines into runs and styles each run per kind (keyword, string, comment), keeping the row's add/remove background. Selected rows keep their fill via the wash helper. No changes to the diff data model.</div>
          </div>
          <div style={{ flexShrink: 0 }}>
            <div style={{ font: "600 11px/14px var(--font-system)", color: "var(--label-tertiary)", marginBottom: 10 }}>CONVERSATION</div>
            <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
              <div style={{ border: "0.5px solid var(--control-border)", borderRadius: 10, padding: "12px 14px", display: "flex", flexDirection: "column", gap: 12 }}>
                <PRComment av="CJ" who="craig" t="2h ago" body="Approved from nat — the wash helper fix looks right. One thing: does runComment win over runString inside backtick strings?" />
                <PRComment av="✦" who="agent" t="2h ago" nested body={<>Strings are tokenised first, so backtick contents stay runString — added a case for it in <span style={{ font: "var(--font-code)", fontSize: 12 }}>diffsyntax_test.go:141</span>.</>} />
                <PRComposer compact placeholder="Reply to this thread…" />
              </div>
            </div>
          </div>
          </div>
          <div style={{ padding: "10px 22px 14px", borderTop: "0.5px solid var(--separator)", flexShrink: 0 }}>
            <PRComposer placeholder="Leave a comment on the pull request…" />
          </div>
        </div>
        <div style={{ width: 216, flexShrink: 0, borderLeft: "0.5px solid var(--separator)", overflowY: "auto", padding: "18px 14px", display: "flex", flexDirection: "column", gap: 16 }}>
          <div>
            <div style={{ font: "600 11px/14px var(--font-system)", color: "var(--label-tertiary)", marginBottom: 8 }}>CHECKS · 2 OF 3</div>
            {[["checkmark_circle_fill", "var(--system-green)", "build · macos-14", "1m 24s"], ["checkmark_circle_fill", "var(--system-green)", "test · go 1.23", "2m 08s"], ["circle_lefthalf_fill", "var(--system-orange)", "lint · golangci", "running…", true]].map(([ic, c, l, r, pulse]) => (
              <div key={l} style={{ display: "flex", alignItems: "center", gap: 8, height: 26 }}>
                <span className={pulse ? "ws-pulse" : ""} style={{ display: "inline-flex" }}><WIcon name={ic} size={12} color={c} style={{ verticalAlign: 0 }} /></span>
                <span style={{ font: "var(--font-code)", fontSize: 11, flex: 1, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{l}</span>
                <span style={{ font: "var(--font-caption2)", color: "var(--label-tertiary)", fontVariantNumeric: "tabular-nums" }}>{r}</span>
              </div>
            ))}
          </div>
          <div>
            <div style={{ font: "600 11px/14px var(--font-system)", color: "var(--label-tertiary)", marginBottom: 8 }}>REVIEW</div>
            <div style={{ display: "flex", alignItems: "center", gap: 8, height: 26 }}>
              <WIcon name="checkmark_circle_fill" size={12} color="var(--system-green)" style={{ verticalAlign: 0 }} />
              <span style={{ font: "var(--font-subheadline)" }}>Approved by craig</span>
            </div>
            <div style={{ font: "var(--font-caption2)", color: "var(--label-tertiary)", marginLeft: 20 }}>2 comments resolved in <span style={{ fontFamily: "var(--font-mono)" }}>e3d19c4</span></div>
            <div style={{ display: "flex", alignItems: "center", gap: 8, height: 26, marginTop: 2 }}>
              <WIcon name="plus_circle" size={12} color="var(--label-tertiary)" style={{ verticalAlign: 0 }} />
              <span style={{ font: "var(--font-subheadline)", color: "var(--label-secondary)" }}>Add Reviewer…</span>
            </div>
          </div>
          <div>
            <div style={{ font: "600 11px/14px var(--font-system)", color: "var(--label-tertiary)", marginBottom: 8 }}>CHANGES</div>
            <div style={{ font: "var(--font-subheadline)", color: "var(--label-secondary)", fontVariantNumeric: "tabular-nums", lineHeight: "20px" }}><span style={{ color: "var(--system-green)" }}>+370</span> <span style={{ color: "var(--system-red)" }}>−17</span> · 6 files<br />3 commits on <span style={{ fontFamily: "var(--font-mono)", fontSize: 11 }}>nat/diff-syntax</span></div>
          </div>
        </div>
      </div>
      <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "8px 14px", borderTop: "0.5px solid var(--separator)", flexShrink: 0 }}>
        <span className="ws-pulse" style={{ color: "var(--system-orange)", fontSize: 9 }}>●</span>
        <span style={{ font: "var(--font-subheadline)", color: "var(--label-tertiary)" }}>Waiting on lint — merges when green</span>
        <div style={{ flex: 1 }}></div>
        <WBtn size="small" variant="borderless"><span style={{ display: "inline-flex", alignItems: "center", gap: 5 }}>Open in GitHub<WIcon name="arrow_up_right_square" size={11} color="currentColor" style={{ verticalAlign: 0 }} /></span></WBtn>
        <WBtn variant="prominent">Merge</WBtn>
      </div>
    </VShell>
  );
}

Object.assign(window, { AgentView, BriefView, DiffView, AgentTerminalView, PRView });

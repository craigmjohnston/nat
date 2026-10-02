import SwiftUI
import NatKit

/// The launch popover's own form: a model picker and an effort picker, the
/// same shape, over `AgentOptions`. A view of its own rather than a method on
/// `BriefTabView` so it can be drawn without the popover it normally opens
/// in — a gallery story has no way to capture a real `NSPopover`'s own
/// window, but the content itself is exactly this.
struct LaunchOptionsForm: View {
    @Binding var model: String
    @Binding var effort: String
    let agentOptions: AgentOptions

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            // Model — the effort field's own picker shape, plus Custom for a
            // full model ID: there is no API to enumerate every alias, so
            // `agentOptions.models` is a documented set rather than
            // everything this field allows.
            HStack(alignment: .top, spacing: 8) {
                Text("Model")
                    .font(.system(size: Typo.subhead, weight: .semibold))
                    .frame(width: 50, alignment: .leading)

                ModelPicker(value: $model, options: agentOptions.models) { text in
                    TextField("claude-…", text: text)
                        .textFieldStyle(.roundedBorder)
                        .font(Typo.mono(size: Typo.code))
                }
                .frame(maxWidth: .infinity)
            }

            Divider()
                .padding(.vertical, 4)

            // Effort selector — a fixed set the CLI rejects anything outside
            // of, read from the same source the model field's placeholder is.
            HStack(spacing: 8) {
                Text("Effort")
                    .font(.system(size: Typo.subhead, weight: .semibold))
                    .frame(width: 50, alignment: .leading)

                Picker("Effort", selection: $effort) {
                    Text("Default").tag("")
                    ForEach(agentOptions.efforts, id: \.self) { level in
                        Text(level).tag(level)
                    }
                }
                .pickerStyle(.menu)
                .frame(maxWidth: .infinity, alignment: .trailing)
            }

            Divider()
                .padding(.vertical, 4)

            // Footnote
            Text("Runs detached in tmux — closing nat won't stop it.")
                .font(.system(size: Typo.caption, weight: .regular))
                .ink(.tertiary)
        }
        .frame(width: 280)
    }
}

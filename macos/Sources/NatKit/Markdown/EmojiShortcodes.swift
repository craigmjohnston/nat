import Foundation

/// GitHub's `:shortcode:` emoji, rendered the way GitHub renders them: the
/// name between two colons becomes the emoji it stands for. Only the common
/// set is carried — the faces, hands and marks reviewers actually type — and
/// a name outside it is left exactly as written, colons and all, which is
/// also how GitHub shows a shortcode it does not know.
///
/// A name is GitHub's own alphabet: lowercase letters, digits, `_`, `+` and
/// `-`. A colon that opens no known name is stepped over one character at a
/// time rather than skipped with the name after it, so `10:30:tada:` still
/// finds `:tada:` behind the `:30:` that is not one.
public func replacingEmojiShortcodes(_ text: String) -> String {
    guard text.contains(":") else { return text }
    var out = ""
    var rest = Substring(text)
    while let open = rest.firstIndex(of: ":") {
        out += rest[..<open]
        let after = rest.index(after: open)
        let name = rest[after...].prefix(while: isShortcodeCharacter)
        if !name.isEmpty, name.endIndex < rest.endIndex, rest[name.endIndex] == ":",
           let emoji = emojiShortcodes[String(name)] {
            out += emoji
            rest = rest[rest.index(after: name.endIndex)...]
        } else {
            out += ":"
            rest = rest[after...]
        }
    }
    out += rest
    return out
}

private func isShortcodeCharacter(_ c: Character) -> Bool {
    c.isASCII && (c.isLowercase || c.isNumber || c == "_" || c == "+" || c == "-")
}

/// The common set, by GitHub's own names.
let emojiShortcodes: [String: String] = [
    // Faces
    "smile": "😄", "smiley": "😃", "grinning": "😀", "grin": "😁", "laughing": "😆",
    "satisfied": "😆", "joy": "😂", "rofl": "🤣", "sweat_smile": "😅", "slightly_smiling_face": "🙂",
    "upside_down_face": "🙃", "wink": "😉", "blush": "😊", "innocent": "😇", "heart_eyes": "😍",
    "star_struck": "🤩", "kissing_heart": "😘", "yum": "😋", "stuck_out_tongue": "😛",
    "stuck_out_tongue_winking_eye": "😜", "thinking": "🤔", "neutral_face": "😐",
    "expressionless": "😑", "no_mouth": "😶", "smirk": "😏", "unamused": "😒", "roll_eyes": "🙄",
    "grimacing": "😬", "relieved": "😌", "pensive": "😔", "sleepy": "😪", "sleeping": "😴",
    "mask": "😷", "face_with_head_bandage": "🤕", "nauseated_face": "🤢", "exploding_head": "🤯",
    "sunglasses": "😎", "nerd_face": "🤓", "confused": "😕", "worried": "😟",
    "slightly_frowning_face": "🙁", "open_mouth": "😮", "hushed": "😯", "astonished": "😲",
    "flushed": "😳", "pleading_face": "🥺", "fearful": "😨", "cold_sweat": "😰", "cry": "😢",
    "sob": "😭", "scream": "😱", "confounded": "😖", "disappointed": "😞", "sweat": "😓",
    "weary": "😩", "tired_face": "😫", "triumph": "😤", "rage": "😡", "angry": "😠",
    "skull": "💀", "poop": "💩", "hankey": "💩", "clown_face": "🤡", "ghost": "👻",
    "alien": "👽", "robot": "🤖", "see_no_evil": "🙈", "hear_no_evil": "🙉", "speak_no_evil": "🙊",
    "partying_face": "🥳", "hugs": "🤗", "shushing_face": "🤫", "zipper_mouth_face": "🤐",
    "face_with_monocle": "🧐", "melting_face": "🫠", "saluting_face": "🫡",
    // Hands and people
    "+1": "👍", "thumbsup": "👍", "-1": "👎", "thumbsdown": "👎", "ok_hand": "👌", "wave": "👋",
    "clap": "👏", "raised_hands": "🙌", "pray": "🙏", "muscle": "💪", "point_up": "☝️",
    "point_down": "👇", "point_left": "👈", "point_right": "👉", "v": "✌️", "crossed_fingers": "🤞",
    "metal": "🤘", "call_me_hand": "🤙", "raised_hand": "✋", "hand": "✋", "fist": "✊",
    "facepunch": "👊", "punch": "👊", "handshake": "🤝", "writing_hand": "✍️", "eyes": "👀",
    "eye": "👁️", "brain": "🧠", "shrug": "🤷", "facepalm": "🤦", "bow": "🙇",
    // Hearts and marks
    "heart": "❤️", "orange_heart": "🧡", "yellow_heart": "💛", "green_heart": "💚",
    "blue_heart": "💙", "purple_heart": "💜", "black_heart": "🖤", "white_heart": "🤍",
    "broken_heart": "💔", "sparkling_heart": "💖", "100": "💯", "boom": "💥", "collision": "💥",
    "sparkles": "✨", "star": "⭐", "star2": "🌟", "dizzy": "💫", "zap": "⚡", "fire": "🔥",
    "tada": "🎉", "confetti_ball": "🎊", "balloon": "🎈", "gift": "🎁", "trophy": "🏆",
    "medal_sports": "🏅", "1st_place_medal": "🥇", "rocket": "🚀", "white_check_mark": "✅",
    "heavy_check_mark": "✔️", "ballot_box_with_check": "☑️", "x": "❌", "negative_squared_cross_mark": "❎",
    "heavy_multiplication_x": "✖️", "warning": "⚠️", "no_entry": "⛔", "no_entry_sign": "🚫",
    "stop_sign": "🛑", "question": "❓", "grey_question": "❔", "exclamation": "❗",
    "heavy_exclamation_mark": "❗", "grey_exclamation": "❕", "bangbang": "‼️", "interrobang": "⁉️",
    "information_source": "ℹ️", "heavy_plus_sign": "➕", "heavy_minus_sign": "➖",
    "arrow_right": "➡️", "arrow_left": "⬅️", "arrow_up": "⬆️", "arrow_down": "⬇️",
    "arrows_counterclockwise": "🔄", "repeat": "🔁", "red_circle": "🔴", "orange_circle": "🟠",
    "yellow_circle": "🟡", "green_circle": "🟢", "large_blue_circle": "🔵", "white_circle": "⚪",
    "black_circle": "⚫", "new": "🆕", "ok": "🆗", "cool": "🆒", "soon": "🔜", "top": "🔝",
    // Things reviewers point at
    "bug": "🐛", "construction": "🚧", "wrench": "🔧", "hammer": "🔨", "hammer_and_wrench": "🛠️",
    "gear": "⚙️", "lock": "🔒", "unlock": "🔓", "key": "🔑", "mag": "🔍", "mag_right": "🔎",
    "bulb": "💡", "memo": "📝", "pencil": "📝", "pencil2": "✏️", "book": "📖", "books": "📚",
    "bookmark": "🔖", "pushpin": "📌", "round_pushpin": "📍", "paperclip": "📎", "link": "🔗",
    "package": "📦", "inbox_tray": "📥", "outbox_tray": "📤", "email": "📧", "envelope": "✉️",
    "bell": "🔔", "no_bell": "🔕", "speech_balloon": "💬", "thought_balloon": "💭",
    "chart_with_upwards_trend": "📈", "chart_with_downwards_trend": "📉", "bar_chart": "📊",
    "clipboard": "📋", "calendar": "📆", "date": "📅", "hourglass": "⌛", "hourglass_flowing_sand": "⏳",
    "stopwatch": "⏱️", "alarm_clock": "⏰", "watch": "⌚", "computer": "💻", "desktop_computer": "🖥️",
    "keyboard": "⌨️", "iphone": "📱", "floppy_disk": "💾", "cd": "💿", "battery": "🔋",
    "electric_plug": "🔌", "test_tube": "🧪", "microscope": "🔬", "telescope": "🔭",
    "art": "🎨", "lipstick": "💄", "recycle": "♻️", "wastebasket": "🗑️", "fire_engine": "🚒",
    "rotating_light": "🚨", "ambulance": "🚑", "label": "🏷️", "triangular_flag_on_post": "🚩",
    "checkered_flag": "🏁", "white_flag": "🏳️", "goal_net": "🥅", "dart": "🎯", "game_die": "🎲",
    "jigsaw": "🧩", "magic_wand": "🪄", "crystal_ball": "🔮", "money_with_wings": "💸",
    "moneybag": "💰", "gem": "💎", "crown": "👑", "sunny": "☀️", "cloud": "☁️", "umbrella": "☂️",
    "snowflake": "❄️", "rainbow": "🌈", "ocean": "🌊", "seedling": "🌱", "herb": "🌿",
    "four_leaf_clover": "🍀", "cactus": "🌵", "evergreen_tree": "🌲", "fallen_leaf": "🍂",
    "coffee": "☕", "tea": "🍵", "beer": "🍺", "beers": "🍻", "pizza": "🍕", "cake": "🍰",
    "cookie": "🍪", "popcorn": "🍿", "hot_pepper": "🌶️", "lemon": "🍋", "apple": "🍎",
    "dog": "🐶", "cat": "🐱", "mouse": "🐭", "rabbit": "🐰", "fox_face": "🦊", "bear": "🐻",
    "panda_face": "🐼", "koala": "🐨", "tiger": "🐯", "lion": "🦁", "cow": "🐮", "pig": "🐷",
    "frog": "🐸", "monkey_face": "🐵", "chicken": "🐔", "penguin": "🐧", "bird": "🐦",
    "baby_chick": "🐤", "owl": "🦉", "unicorn": "🦄", "bee": "🐝", "honeybee": "🐝", "ant": "🐜",
    "snail": "🐌", "butterfly": "🦋", "turtle": "🐢", "snake": "🐍", "octopus": "🐙",
    "crab": "🦀", "whale": "🐳", "dolphin": "🐬", "fish": "🐟", "shark": "🦈",
    "sloth": "🦥", "otter": "🦦", "hedgehog": "🦔", "t-rex": "🦖", "sauropod": "🦕",
    "dragon": "🐉", "ship": "🚢", "boat": "⛵", "sailboat": "⛵", "car": "🚗", "taxi": "🚕",
    "bus": "🚌", "train": "🚆", "bike": "🚲", "airplane": "✈️", "helicopter": "🚁",
    "house": "🏠", "office": "🏢", "earth_americas": "🌎", "globe_with_meridians": "🌐",
    "zzz": "💤", "sweat_drops": "💦", "dash": "💨", "musical_note": "🎵", "notes": "🎶",
    "headphones": "🎧", "microphone": "🎤", "video_game": "🎮", "camera": "📷", "movie_camera": "🎥",
    "tv": "📺", "radio": "📻",
]

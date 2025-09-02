from pytdbot import Client, types
from pytdbot.exception import StopHandlers


@Client.on_message()
async def start(c: Client, message: types.Message):
    if message.text != "/start":
        raise StopHandlers

    keyboard = [
        [
            types.InlineKeyboardButton("📚 Documentation",
                                       type=types.InlineKeyboardButtonTypeUrl("https://pytgcalls.github.io/"))
        ],
        [
            types.InlineKeyboardButton("🔍 Search", type=types.InlineKeyboardButtonTypeSwitchInline(query="Quick start",
                                                                                                   target_chat=types.TargetChatCurrent())),
        ]
    ]

    welcome_text = """👋 <b>Welcome to PyTgCalls Documentation Bot!</b>

I can help you find information about PyTgCalls and NTgCalls methods, classes, and more.

• Use the 🔍 <b>Search</b> button to search the documentation
• Or type your query directly in the chat
• Visit our <a href="https://pytgcalls.github.io/">Documentation</a> for detailed guides

Made with ❤️ by @AshokShau"""
    ok = await message.reply_text(
        text=welcome_text,
        reply_markup=types.ReplyMarkupInlineKeyboard(keyboard),
        disable_web_page_preview=True
    )
    if isinstance(ok, types.Error):
        c.logger.warning(f"Failed to send start message: {ok.message}")

    raise StopHandlers

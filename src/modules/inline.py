import html
import uuid

from pytdbot import Client, types

from .search import DocSearch


async def format_doc_info(r, include_raises: bool = False) -> str:
    parts = [f"<b>{html.escape(r.title)}</b> <i>({r.kind}, {r.lib})</i>"]

    # signature
    if r.details.signature:
        parts.append(f"<pre>{html.escape(r.details.signature)}</pre>")

    # description
    if r.description:
        parts.append(html.escape(r.description))

    # example code
    if r.example and r.example.get("code"):
        code = html.escape(r.example["code"].strip())
        lang = r.example.get("language", "")
        parts.append(f"<b>Example ({lang}):</b>\n<pre>{code}</pre>")

    # sections (PARAMETERS etc., skip RAISES unless include_raises=True)
    if r.details.sections:
        for s in r.details.sections:
            if s.title.upper() == "RAISES" and not include_raises:
                continue
            parts.append(f"<b>{html.escape(s.title)}</b>")
            for it in s.items:
                nm = it.get("name") or ""
                tp = it.get("type") or ""
                ds = (it.get("description") or "").strip()

                if s.title.upper() == "RAISES":
                    for line in ds.split("\n"):
                        line = line.strip()
                        if line:
                            parts.append(f"• {html.escape(line)}")
                else:
                    param_line = f"<code>{html.escape(nm)}</code>"
                    if tp:
                        param_line += f": <i>{html.escape(tp)}</i>"
                    if ds:
                        param_line += f" — {html.escape(ds)}"
                    parts.append("• " + param_line)

    # members (for enums/types)
    if r.details.members:
        parts.append("<b>Members:</b>")
        for m in r.details.members:
            line = f"<code>{html.escape(m.name)}</code>"
            if m.value:
                line += f" = <code>{html.escape(m.value)}</code>"
            if m.description:
                line += f" — {html.escape(m.description)}"
            parts.append("• " + line)

    return "\n".join(parts)

searcher = DocSearch("docs.json")

_thumb_url = "https://raw.githubusercontent.com/pytgcalls/pytgcalls.github.io/c732cc3b58002ddcf96eab7e44d3180448445bc5/src/assets/pytgcalls.svg"

@Client.on_updateNewInlineQuery()
async def inline_search(c: Client, message: types.UpdateNewInlineQuery):
    query = message.query.strip()
    if not query:
        return None

    search_results = searcher.search(query, limit=10)
    if not search_results:
        await c.answerInlineQuery(
            message.id,
            results=[
                types.InputInlineQueryResultArticle(
                    id=str(uuid.uuid4()),
                    title="❌ No Results Found",
                    description="No documentation found for your query.",
                    thumbnail_url=_thumb_url,
                    input_message_content=types.InputMessageText(
                        text=types.FormattedText(f"No documentation found for: {query}")
                    ),
                )
            ]
        )
        return None

    results = []
    for r in search_results:
        full_doc = await format_doc_info(r)
        if len(full_doc) > 4096:
            c.logger.warning(f"Document {r.title} is too long to be sent inline.")
            continue

        text = await c.parseTextEntities(full_doc, types.TextParseModeHTML())
        if isinstance(text, types.Error):
            c.logger.warning(f"❌ Error parsing inline result for {r.title}: {text.message}")
            continue

        result = types.InputInlineQueryResultArticle(
            id=str(uuid.uuid4()),
            title=r.title + " " +f"({r.lib})",
            description=r.description[:100] + "...",
            input_message_content=types.InputMessageText(text=text),
            thumbnail_url=_thumb_url,
            reply_markup=types.ReplyMarkupInlineKeyboard([
                [
                    types.InlineKeyboardButton(
                        text="📚 View Full Documentation",
                        type=types.InlineKeyboardButtonTypeUrl(r.doc_url)
                    )
                ]
            ])
        )
        results.append(result)

    if not results:
        await c.answerInlineQuery(
            message.id,
            results=[
                types.InputInlineQueryResultArticle(
                    id=str(uuid.uuid4()),
                    title="❌ Something went wrong",
                    description="No documentation found for your query.",
                    thumbnail_url=_thumb_url,
                    input_message_content=types.InputMessageText(
                        text=types.FormattedText(f"No documentation found for: {query}")
                    ),
                )
            ]
        )
        return None

    ok = await c.answerInlineQuery(message.id, results=results)
    if isinstance(ok, types.Error):
        c.logger.warning(f"Failed to send inline response: {ok.message}")
    return None

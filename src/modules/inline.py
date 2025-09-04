import uuid

from pytdbot import Client, types

from ._format import format_doc_info
from .search import DocSearch

searcher = DocSearch("docs.json")
_thumb_url = "https://avatars.githubusercontent.com/u/75855609?s=200&v=4"

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

import uuid

from pytdbot import Client, types

from ._format import format_doc_info, keyboard
from .search import DocSearch

searcher = DocSearch("docs.json")
_thumb_url = "https://avatars.githubusercontent.com/u/75855609?s=200&v=4"
_RESULTS_PER_PAGE = 10

@Client.on_updateNewInlineQuery()
async def inline_search(c: Client, message: types.UpdateNewInlineQuery):
    query = message.query.strip()
    if not query:
        text = await c.parseTextEntities("Click the button below to search the documentation.", types.TextParseModeHTML())
        await c.answerInlineQuery(
            message.id,
            results=[
                types.InputInlineQueryResultArticle(
                    id=str(uuid.uuid4()),
                    title="🔍 Search Documentation",
                    description="Type something to search the docs.",
                    thumbnail_url=_thumb_url,
                    input_message_content=types.InputMessageText(text=text),
                    reply_markup=types.ReplyMarkupInlineKeyboard(keyboard),
                )
            ],
            cache_time=60,
            button=types.InlineQueryResultsButton(text="🔍 Documentation", type=types.InlineQueryResultsButtonTypeWebApp(url="https://pytgcalls.github.io/")),
            is_personal=True
        )
        return None
        
    offset = int(message.offset) if message.offset else 0
    all_results = searcher.search(query, limit=50)
    
    if not all_results:
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
            ],
            next_offset="",
            cache_time=300,
            is_personal=True
        )
        return None
    
    # Paginate results
    start_idx = offset
    end_idx = offset + _RESULTS_PER_PAGE
    paginated_results = all_results[start_idx:end_idx]
    next_offset = str(offset + _RESULTS_PER_PAGE) if end_idx < len(all_results) else ""
    
    results = []
    for r in paginated_results:
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
            title=f"{r.title} " + f"({r.lib})",
            description=r.description[:100] if hasattr(r, 'description') else '',
            thumbnail_url=_thumb_url,
            input_message_content=types.InputMessageText(text=text),
            reply_markup=types.ReplyMarkupInlineKeyboard(
                [
                    [
                        types.InlineKeyboardButton(
                            text="📚 View Full Documentation",
                            type=types.InlineKeyboardButtonTypeUrl(r.doc_url),
                        )
                    ]
                ]
            ),
        )
        results.append(result)
    
    await c.answerInlineQuery(
        message.id,
        results=results,
        next_offset=next_offset,
        cache_time=300,
        is_personal=True
    )
    return None

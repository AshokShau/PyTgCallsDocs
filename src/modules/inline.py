import re
import uuid

from pytdbot import Client, types

from ._format import format_doc_info, keyboard, replace_with_doc_links
from .search import DocSearch

# Initialize components
searcher = DocSearch("docs.json")
_thumb_url = "https://avatars.githubusercontent.com/u/75855609?s=200&v=4"
_RESULTS_PER_PAGE = 10



@Client.on_updateNewInlineQuery()
async def inline_search(client: Client, update: types.UpdateNewInlineQuery):
    query = update.query.strip()

    if not query:
        await _handle_empty_query(client, update)
        return

    # Check for special pattern matching (e.g., +function+)
    if re.search(r"\+([A-Za-z0-9 _-]+)\+", query):
        await _handle_pattern_query(client, update, query)
        return

    # Handle regular search query
    await _handle_regular_query(client, update, query)


async def _handle_empty_query(client: Client, update: types.UpdateNewInlineQuery):
    """Handle empty query with welcome message."""
    welcome_text = await client.parseTextEntities(
        "Click the button below to search the documentation.",
        types.TextParseModeHTML()
    )

    result = types.InputInlineQueryResultArticle(
        id=str(uuid.uuid4()),
        title="🔍 Search Documentation",
        description="Type something to search the docs.",
        thumbnail_url=_thumb_url,
        input_message_content=types.InputMessageText(text=welcome_text),
        reply_markup=types.ReplyMarkupInlineKeyboard(keyboard),
    )

    webapp_button = types.InlineQueryResultsButton(
        text="🔍 Documentation",
        type=types.InlineQueryResultsButtonTypeWebApp(
            url="https://pytgcalls.github.io/"
        )
    )

    await client.answerInlineQuery(
        update.id,
        results=[result],
        cache_time=60,
        button=webapp_button,
        is_personal=True
    )


async def _handle_pattern_query(client: Client, update: types.UpdateNewInlineQuery, query: str):
    """Handle queries with special pattern syntax (e.g., +function+)."""
    doc_links = await replace_with_doc_links(query, searcher)
    if not doc_links:
        client.logger.info(f"No results found for pattern query: {query}")
        return

    results = []
    for link in doc_links:
        if not link.title:
            continue

        result = types.InputInlineQueryResultArticle(
            id=str(uuid.uuid4()),
            title=link.title,
            description=link.title,
            thumbnail_url=_thumb_url,
            input_message_content=types.InputMessageText(
                text=await client.parseTextEntities(link.result_text, types.TextParseModeHTML())
            ),
        )
        results.append(result)

    response = await client.answerInlineQuery(
        update.id,
        results=results,
        cache_time=300,
        is_personal=True
    )

    if isinstance(response, types.Error):
        client.logger.warning(f"Failed to send inline results: {response.message}")


async def _handle_regular_query(client: Client, update: types.UpdateNewInlineQuery, query: str):
    """Handle regular search queries."""
    offset = int(update.offset) if update.offset else 0
    all_results = searcher.search(query, limit=50)

    if not all_results:
        await _send_no_results_response(client, update, query)
        return

    # Paginate results
    start_idx = offset
    end_idx = offset + _RESULTS_PER_PAGE
    paginated_results = all_results[start_idx:end_idx]
    next_offset = str(offset + _RESULTS_PER_PAGE) if end_idx < len(all_results) else ""

    results = []
    for result in paginated_results:
        if inline_result := await _create_inline_result(client, result):
            results.append(inline_result)

    if not results:
        await _send_no_results_response(client, update, query)
        return

    ok = await client.answerInlineQuery(
        update.id,
        results=results,
        next_offset=next_offset,
        cache_time=300,
        is_personal=True
    )
    if isinstance(ok, types.Error):
        client.logger.warning(f"Failed to send inline results: {ok.message}")


async def _create_inline_result(client: Client, search_result) -> types.InputInlineQueryResultArticle | None:
    """Create an inline result from a search result."""
    full_doc = await format_doc_info(search_result)

    # Check content length limit
    if len(full_doc) > 4096:
        client.logger.warning(f"Document {search_result.title} is too long to be sent inline")
        return None

    # Parse text entities
    parsed_text = await client.parseTextEntities(full_doc, types.TextParseModeHTML())
    if isinstance(parsed_text, types.Error):
        client.logger.warning(f"Error parsing inline result for {search_result.title}: {parsed_text.message}")
        return None

    # Create result
    return types.InputInlineQueryResultArticle(
        id=str(uuid.uuid4()),
        title=f"{search_result.title} ({search_result.lib})",
        description=search_result.description[:100],
        thumbnail_url=_thumb_url,
        input_message_content=types.InputMessageText(text=parsed_text),
        reply_markup=types.ReplyMarkupInlineKeyboard([
            [
                types.InlineKeyboardButton(
                    text="📚 View Full Documentation",
                    type=types.InlineKeyboardButtonTypeUrl(search_result.doc_url),
                )
            ]
        ]),
    )


async def _send_no_results_response(client: Client, update: types.UpdateNewInlineQuery, query: str):
    """Send a response when no results are found."""
    no_results = types.InputInlineQueryResultArticle(
        id=str(uuid.uuid4()),
        title="❌ No Results Found",
        description="No documentation found for your query.",
        thumbnail_url=_thumb_url,
        input_message_content=types.InputMessageText(
            text=types.FormattedText(f"No documentation found for: {query}")
        ),
    )

    await client.answerInlineQuery(
        update.id,
        results=[no_results],
        next_offset="",
        cache_time=300,
        is_personal=True
    )

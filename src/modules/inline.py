import html
import os
import re
import uuid

from pytdbot import Client, types

from src.core.search import Search

# Initialize the search engine
BASE_DIR = os.path.join(os.path.dirname(os.path.dirname(os.path.dirname(__file__))), 'docsdata')
search_engine = Search(BASE_DIR)


async def format_doc_info(path: str) -> str:
    full_info = await search_engine.get_path_full_info(path)
    if not full_info:
        return ""

    result = f"<b>{html.escape(full_info.title)}</b>\n\n"
    
    # Add description if available
    if full_info.description and full_info.description.strip() != full_info.title:
        result += f"{html.escape(full_info.description.strip())}\n\n"
    
    # Add details if available and different from description
    if full_info.details and full_info.details.strip() not in [full_info.description, full_info.title]:
        result += f"{html.escape(full_info.details.strip())}\n\n"
    
    # Add Enum members section if this is an Enum type
    if full_info.enum_members:
        result += "<b>📊 Enum Members</b>\n"
        for member in full_info.enum_members:
            member_info = f"• <code>{html.escape(member.name)}</code>"
            if member.value:
                member_info += f" = {html.escape(member.value)}"
            if member.description:
                member_info += f": {html.escape(member.description)}"
            result += f"{member_info}\n"
        result += "\n"
    
    # Add parameters section if we have any parameters
    if full_info.parameters and any(p.name.strip() for p in full_info.parameters):
        result += "<b>📝 Parameters</b>\n"
        for param in full_info.parameters:
            if not param.name.strip():
                continue
                
            param_info = f"• <code>{html.escape(param.name.strip())}</code>"
            
            # Add type if available
            if param.type and param.type.strip():
                param_info += f" ({html.escape(param.type.strip())})"
            
            # Add description if available and not just a repeat of the name
            if (param.description and 
                param.description.strip().lower() != param.name.strip().lower()):
                param_info += f": {html.escape(param.description.strip())}"
                
            result += f"{param_info}\n"
        result += "\n"
    
    # Add examples section if available
    if full_info.examples:
        result += "<b>💡 Example</b>\n" if len(full_info.examples) == 1 else "<b>💡 Examples</b>\n"
        
        for example in full_info.examples:
            if not example or not example.strip():
                continue
                
            # Clean up the example code
            lines = [line.rstrip() for line in example.split('\n') if line.strip()]
            if not lines:
                continue
                
            # Find minimum indentation (skip empty lines and comment-only lines)
            min_indent = min(
                len(line) - len(line.lstrip())
                for line in lines
                if line.strip() and not line.lstrip().startswith('#')
            )
            
            # Format each line with proper indentation
            formatted_lines = []
            for line in lines:
                if line.strip():
                    # Remove common indentation and add 4 spaces
                    formatted_line = line[min_indent:]
                    # Preserve empty lines
                    if not formatted_line.strip():
                        formatted_lines.append('')
                    else:
                        formatted_lines.append(formatted_line)
            
            # Join with newlines and add to result
            formatted_example = '\n'.join(formatted_lines)
            result += f"<pre><code class=\"python\">{html.escape(formatted_example)}</code></pre>\n\n"
    
    return result.strip()


@Client.on_updateNewInlineQuery()
async def inline_search(c: Client, message: types.UpdateNewInlineQuery):
    query = message.query.strip()
    if not query:
        return None

    search_results = search_engine.search(query, limit=20)
    c.logger.info(f"Search results: {search_results} for query: {query}")
    
    if not search_results:
        ok = await c.answerInlineQuery(
            message.id,
            results=[
                types.InputInlineQueryResultArticle(
                    id=str(uuid.uuid4()),
                    title="❌ No Results Found",
                    description="No documentation found for your query.",
                    input_message_content=types.InputMessageText(
                        text=types.FormattedText(f"No documentation found for: {query}")
                    )
                )
            ]
        )
        if isinstance(ok, types.Error):
            c.logger.warning(f"Failed to send inline response: {ok.message}")
        return None

    results = []
    for result in search_results:
        try:
            full_doc = await format_doc_info(result.path)
            if not full_doc:
                continue
                
            if len(full_doc) > 4096:
                full_doc = full_doc[:4000] + "...\n\n<i>Documentation was truncated due to length. View full documentation for complete details.</i>"

            preview = result.preview[:100] + "..." if result.preview else result.title
            doc_path = result.path.strip('/')
            is_ntgcalls = 'ntgcalls' in doc_path.lower()
            
            if is_ntgcalls and doc_path.startswith('NTgCalls/'):
                doc_path = doc_path[len('NTgCalls/'):]
            elif not is_ntgcalls and doc_path.startswith('PyTgCalls/'):
                doc_path = doc_path[len('PyTgCalls/'):]
                
            if doc_path.endswith('.xml'):
                doc_path = doc_path[:-4]
                
            doc_type = "NTgCalls" if is_ntgcalls else "PyTgCalls"
            doc_url = f"https://pytgcalls.github.io/{doc_type}/{doc_path}"

            parse = await c.parseTextEntities(full_doc, types.TextParseModeHTML())
            if isinstance(parse, types.Error):
                c.logger.warning(f"❌ Error parsing inline result for {result.title}: {parse.message}")
                continue

            result = types.InputInlineQueryResultArticle(
                id=str(uuid.uuid4()),
                title=result.title,
                description=re.sub(r"\*{1,2}(.*?)\*{1,2}", r"\1", preview),
                input_message_content=types.InputMessageText(text=parse),
                reply_markup=types.ReplyMarkupInlineKeyboard([
                    [
                        types.InlineKeyboardButton(
                            text="📚 View Full Documentation",
                            type=types.InlineKeyboardButtonTypeUrl(doc_url)
                        )
                    ]
                ])
            )
            results.append(result)
            
        except Exception as e:
            c.logger.error(f"Error processing search result: {e}", exc_info=True)
            continue

    if not results:
        return None

    ok = await c.answerInlineQuery(message.id, results=results)
    if isinstance(ok, types.Error):
        c.logger.warning(f"Failed to send inline response: {ok.message}")
    return None

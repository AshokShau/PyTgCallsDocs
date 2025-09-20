import html
import re
import typing as t
from dataclasses import dataclass

from pytdbot import types

if t.TYPE_CHECKING:
    from .search import DocSearch


@dataclass
class DocsLink:
    title: str
    result_text: str


async def replace_with_doc_links(text: str, searcher: "DocSearch") -> t.List[DocsLink]:
    pattern = r"\+([A-Za-z0-9 _-]+)\+"
    matches = list(re.finditer(pattern, text))
    if not matches:
        return []

    unique_names = {m.group(1).strip() for m in matches}
    search_results = {
        name: searcher.search(name, limit=5) for name in unique_names
    }
    docs_links: t.List[DocsLink] = []
    for idx in range(5):
        result_text = ""
        last_end = 0
        found_titles: t.List[str] = []

        for match in matches:
            name = match.group(1).strip()
            result_text += text[last_end:match.start()]

            docs = search_results.get(name, [])
            if docs and idx < len(docs):
                doc = docs[idx]
                replacement = f'<a href="{doc.doc_url}">{doc.title}</a>'
                found_titles.append(f"{doc.title} ({doc.lib})")
            else:
                replacement = f"<code>{name}</code>"

            result_text += replacement
            last_end = match.end()

        result_text += text[last_end:]
        title = " | ".join(found_titles) if found_titles else ""
        docs_links.append(DocsLink(title=title, result_text=result_text))

    return docs_links


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
                        if line := line.strip():
                            parts.append(f"• {html.escape(line)}")
                else:
                    param_line = f"<code>{html.escape(nm)}</code>"
                    if tp:
                        param_line += f": <i>{html.escape(tp)}</i>"
                    if ds:
                        param_line += f" — {html.escape(ds)}"
                    parts.append(f"• {param_line}")

    # members (for enums)
    if r.details.members:
        parts.append("<b>Members:</b>")
        for m in r.details.members:
            line = f"<code>{html.escape(m.name)}</code>"
            if m.value:
                line += f" = <code>{html.escape(m.value)}</code>"
            if m.description:
                line += f" — {html.escape(m.description)}"
            parts.append(f"• {line}")

    # properties (for types)
    if r.details.properties:
        parts.append("<b>Properties:</b>")
        for p in r.details.properties:
            line = f"<code>{html.escape(p.name)}</code>"
            if p.type:
                line += f" -> <i>{html.escape(p.type)}</i>"
            if p.description:
                line += f" — {html.escape(p.description)}"
            parts.append(f"• {line}")

    return "\n".join(parts)


keyboard = [
    [
        types.InlineKeyboardButton(
            text="📚 Documentation",
            type=types.InlineKeyboardButtonTypeUrl("https://pytgcalls.github.io/"))
    ],
    [
        types.InlineKeyboardButton(
            text="🔍 Search",
            type=types.InlineKeyboardButtonTypeSwitchInline(query="Quick start",
                                                            target_chat=types.TargetChatCurrent())),
    ]
]

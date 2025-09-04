import html

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

    # members (for enums)
    if r.details.members:
        parts.append("<b>Members:</b>")
        for m in r.details.members:
            line = f"<code>{html.escape(m.name)}</code>"
            if m.value:
                line += f" = <code>{html.escape(m.value)}</code>"
            if m.description:
                line += f" — {html.escape(m.description)}"
            parts.append("• " + line)

    # properties (for types)
    if r.details.properties:
        parts.append("<b>Properties:</b>")
        for p in r.details.properties:
            line = f"<code>{html.escape(p.name)}</code>"
            if p.type:
                line += f" -> <i>{html.escape(p.type)}</i>"
            if p.description:
                line += f" — {html.escape(p.description)}"
            parts.append("• " + line)

    return "\n".join(parts)

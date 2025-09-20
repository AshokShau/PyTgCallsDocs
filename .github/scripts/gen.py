import json
import re
import textwrap
import urllib.request
import xml.etree.ElementTree as ET
from pathlib import Path
from typing import Optional, Union


class DocItem(dict):
    """Represents a parsed documentation item (field/member)."""

    def __init__(
            self,
            name: str,
            type_: Optional[str] = None,
            description: str = "",
            source_config: Optional[str] = None,
            value: Optional[str] = None,
    ):
        super().__init__(
            name=name,
            type=type_,
            description=description,
            source_config=source_config,
            value=value,
        )


def read_file_or_url(path_or_url: Union[str, Path]) -> str:
    """Read file content from local path or raw GitHub URL."""
    if isinstance(path_or_url, Path) or Path(path_or_url).exists():
        return Path(path_or_url).read_text(encoding="utf-8")
    if str(path_or_url).startswith("http"):
        with urllib.request.urlopen(str(path_or_url)) as resp:
            return resp.read().decode("utf-8")
    raise FileNotFoundError(f"Cannot read {path_or_url}")


# ---------------- XML CONFIG ----------------
def parse_config(config_source: Union[str, Path]):
    xml_text = read_file_or_url(config_source)
    root = ET.fromstring(xml_text)
    config_map = {}

    def resolve(node, seen=None):
        if seen is None:
            seen = set()
        parts = []
        for child in node:
            if child.tag == "config":
                ref_id = child.attrib["id"]
                if ref_id in seen:
                    continue
                seen.add(ref_id)
                ref_node = root.find(f"option[@id='{ref_id}']")
                if ref_node is not None:
                    parts.append(resolve(ref_node, seen))
                else:
                    parts.append(f"[UNRESOLVED:{ref_id}]")
            else:
                parts.append("".join(child.itertext()).strip())
        return "\n".join([p for p in parts if p])

    for option in root.findall("option"):
        cid = option.attrib["id"]
        config_map[cid] = resolve(option)

    return config_map


# ---------------- HELPERS ----------------
def normalize_items(items):
    """Normalize raw XML items (for method parameter sections)."""
    result = []
    for item in items:
        if "resolved" in item:
            text = item["resolved"].strip()
            lines = text.split("\n", 1)
            first_line = lines[0]
            desc = lines[1].strip() if len(lines) > 1 else ""
            if ":" in first_line:
                name, typ = first_line.split(":", 1)
                result.append(DocItem(name.strip(), typ.strip(), desc, item.get("config_id")))
            elif result:
                result[-1]["description"] += f" {text}"
            else:
                result.append(DocItem("", None, text, item.get("config_id")))
        elif "raw" in item:
            name, _, typ = item["raw"].partition(":")
            result.append(DocItem(name.strip(), typ.strip() if typ else None, "", None))
        elif "text" in item:
            if result:
                result[-1]["description"] += (" " + item["text"].strip())
            else:
                result.append(DocItem("", None, item["text"].strip(), None))
    return result


# ---------------- TYPE PAGE PARSER ----------------
def parse_type_page(page, config_map):
    """
    Parse a page representing a type, enum, or Stream Descriptor.
    Returns dict: {signature, members(list), properties(list), parameters(list), description(str)}
    """
    signature = None
    sig = page.find("category-title")
    if sig is not None:
        signature = "".join(sig.itertext()).strip()

    description_parts = []
    members = []
    properties = []
    parameters = []
    current = None

    def handle_member_block(block):
        nonlocal current
        for child in block:
            if child.tag == "category-title":
                raw = "".join(child.itertext()).strip()
                if "=" in raw:
                    name, val = raw.split("=", 1)
                    current = DocItem(name.strip(), None, "", None, val.strip())
                else:
                    current = DocItem(raw.strip(), None, "", None, None)
                members.append(current)
            elif child.tag == "subtext":
                desc_txt = " ".join(t.strip() for t in child.itertext() if t.strip())
                if desc_txt:
                    if current:
                        cur_desc = (current.get("description") or "").strip()
                        current["description"] = (
                            f"{cur_desc} {desc_txt}".strip()
                            if cur_desc
                            else desc_txt
                        )
                    else:
                        description_parts.append(desc_txt)
            elif child.tag == "config":
                cid = child.attrib["id"]
                text = config_map.get(cid, f"[UNRESOLVED:{cid}]")
                if current:
                    cur_desc = (current.get("description") or "").strip()
                    current["description"] = (
                        f"{cur_desc} {text.strip()}".strip()
                        if cur_desc
                        else text.strip()
                    )
                else:
                    description_parts.append(text.strip())
            elif child.tag == "text":
                txt = (child.text or "").strip()
                if txt:
                    if current:
                        cur_desc = (current.get("description") or "").strip()
                        current["description"] = f"{cur_desc} {txt}".strip() if cur_desc else txt
                    else:
                        description_parts.append(txt)

    def handle_property_block(block):
        nonlocal current
        for child in block:
            if child.tag == "category-title":
                raw = "".join(child.itertext()).strip()
                name = raw
                type_text = None
                if "->" in raw:
                    left, right = raw.split("->", 1)
                    name = left.strip()
                    type_text = right.strip()
                else:
                    dr = child.find(".//docs-ref")
                    if dr is not None:
                        type_text = "".join(dr.itertext()).strip()
                current = DocItem(name.strip(), type_text, "", None, None)
                properties.append(current)
            elif child.tag == "subtext":
                desc_txt = " ".join(t.strip() for t in child.itertext() if t.strip())
                if desc_txt:
                    if current:
                        cur_desc = (current.get("description") or "").strip()
                        current["description"] = (
                            f"{cur_desc} {desc_txt}".strip()
                            if cur_desc
                            else desc_txt
                        )
                    else:
                        description_parts.append(desc_txt)
            elif child.tag == "config":
                cid = child.attrib["id"]
                text = config_map.get(cid, f"[UNRESOLVED:{cid}]")
                if current:
                    cur_desc = (current.get("description") or "").strip()
                    current["description"] = (
                        f"{cur_desc} {text.strip()}".strip()
                        if cur_desc
                        else text.strip()
                    )
                else:
                    description_parts.append(text.strip())
            elif child.tag == "text":
                txt = (child.text or "").strip()
                if txt:
                    if current:
                        cur_desc = (current.get("description") or "").strip()
                        current["description"] = f"{cur_desc} {txt}".strip() if cur_desc else txt
                    else:
                        description_parts.append(txt)

    def handle_param_block(block):
        """Handle Stream Descriptor parameters."""
        nonlocal current
        for child in block:
            if child.tag == "category-title":
                raw = "".join(child.itertext()).strip()
                if ":" in raw:
                    name, typ = raw.split(":", 1)
                    current = DocItem(name.strip(), typ.strip(), "", None)
                else:
                    current = DocItem(raw.strip(), None, "", None)
                parameters.append(current)
            elif child.tag == "subtext":
                desc_txt = " ".join(t.strip() for t in child.itertext() if t.strip())
                if desc_txt:
                    if current:
                        cur_desc = (current.get("description") or "").strip()
                        current["description"] = f"{cur_desc} {desc_txt}".strip() if cur_desc else desc_txt
                    else:
                        description_parts.append(desc_txt)
            elif child.tag == "config":
                cid = child.attrib["id"]
                text = config_map.get(cid, f"[UNRESOLVED:{cid}]")
                if current:
                    cur_desc = (current.get("description") or "").strip()
                    current["description"] = f"{cur_desc} {text.strip()}".strip() if cur_desc else text.strip()
                else:
                    description_parts.append(text.strip())
            elif child.tag == "text":
                txt = (child.text or "").strip()
                if txt:
                    if current:
                        cur_desc = (current.get("description") or "").strip()
                        current["description"] = f"{cur_desc} {txt}".strip() if cur_desc else txt
                    else:
                        description_parts.append(txt)

    # walk <subtext> blocks
    for sub in page.findall("subtext"):
        pg = sub.find("pg-title")
        if pg is not None:
            label = "".join(pg.itertext()).upper()
            if "PARAMETERS" in label:
                for inner in sub.findall("subtext"):
                    handle_param_block(inner)
            elif "ENUMERATION MEMBERS" in label:
                for inner in sub.findall("subtext"):
                    handle_member_block(inner)
            elif "PROPERTIES" in label:
                for inner in sub.findall("subtext"):
                    handle_property_block(inner)
            else:
                for child in sub:
                    if child.tag == "config":
                        cid = child.attrib["id"]
                        description_parts.append(config_map.get(cid, f"[UNRESOLVED:{cid}]"))
                    elif child.tag == "text" and child.text:
                        description_parts.append(child.text.strip())
        else:
            for child in sub:
                if child.tag == "config":
                    cid = child.attrib["id"]
                    description_parts.append(config_map.get(cid, f"[UNRESOLVED:{cid}]"))
                elif child.tag == "text" and child.text:
                    description_parts.append(child.text.strip())

    return {
        "signature": signature,
        "members": members,
        "properties": properties,
        "parameters": parameters,
        "description": " ".join(p for p in description_parts if p).strip(),
    }


# ---------------- MAP PARSER ----------------
def parse_map(map_source: Union[str, Path], config_map):
    map_text = read_file_or_url(map_source)
    raw_map = json.loads(map_text)
    docs = {}

    for path, page_xml in raw_map.items():
        page = ET.fromstring(page_xml)
        title = page.findtext("h1", "").strip()

        if path.startswith("/NTgCalls/"):
            lib = "NTgCalls"
            path_suffix = path[len("/NTgCalls/"):]
        elif path.startswith("/PyTgCalls/"):
            lib = "PyTgCalls"
            path_suffix = path[len("/PyTgCalls/"):]
        else:
            lib = "Unknown"
            path_suffix = path

        if "Available Enums" in path:
            kind = "enum"
        elif "Methods" in path:
            kind = "method"
        elif "Available Structs" in path:
            kind = "struct"
        elif "Available Types" in path or "Advanced Types" in path:
            kind = "type"
        elif "Stream Descriptors" in path:
            kind = "descriptor"
        else:
            kind = "misc"

        if kind == "misc":
            def extract_full_description(node):
                parts = []
                for child in node.iter():
                    if child.tag == "config":
                        cid = child.attrib.get("id")
                        if cid:
                            parts.append(config_map.get(cid, f"[UNRESOLVED:{cid}]"))
                    elif child.tag == "text" and child.text:
                        parts.append(child.text.strip())
                return " ".join(parts).strip()

            description = extract_full_description(page)
        else:
            desc_node = page.find("config")
            if desc_node is not None:
                description = config_map.get(desc_node.attrib["id"], "")
            else:
                description = "".join(page.findtext("text", "") or "")

        example = {}
        ex_node = page.find("syntax-highlight")
        if ex_node is not None:
            code_raw = "".join(ex_node.itertext())
            code_clean = textwrap.dedent(code_raw).strip("\n")
            example["language"] = ex_node.attrib.get("language", "python")
            example["code"] = code_clean

        details = {}
        sections = []
        for cat in page.findall(".//category"):
            section_title = cat.findtext("pg-title", "").strip()
            raw_items = []
            for sub in cat.findall("subtext"):
                for item in sub:
                    if item.tag == "config":
                        cid = item.attrib["id"]
                        raw_items.append({
                            "config_id": cid,
                            "resolved": config_map.get(cid, f"[UNRESOLVED:{cid}]")
                        })
                    elif item.tag == "category-title":
                        raw_items.append({"raw": "".join(item.itertext()).strip()})
                    elif item.tag == "text":
                        if item.text:
                            raw_items.append({"text": item.text.strip()})
            if norm_items := normalize_items(raw_items):
                sections.append({"title": section_title, "items": norm_items})

        if sections:
            details["sections"] = sections
        parsed = parse_type_page(page, config_map)
        details["signature"] = parsed["signature"]
        if parsed["members"]:
            details["members"] = parsed["members"]
        if parsed["properties"]:
            details["properties"] = parsed["properties"]
        if parsed["parameters"]:
            details["parameters"] = parsed["parameters"]

        if not description:
            for sub in page.findall("subtext"):
                # always grab <text> even if <pg-title> exists
                for txt_node in sub.findall("text"):
                    if txt_node.text and txt_node.text.strip():
                        description = txt_node.text.strip()
                        break
                # grab <config> if present
                for cfg_node in sub.findall("config"):
                    if cid := cfg_node.attrib.get("id"):
                        description = config_map.get(cid, f"[UNRESOLVED:{cid}]")
                if description:
                    break  # stop after first valid description

        if path_suffix.endswith(".xml"):
            path_suffix = path_suffix[:-4]
        doc_url = f"https://pytgcalls.github.io/{lib}/{path_suffix}"

        description = textwrap.dedent(description).strip("\n")
        description = re.sub(r'\s+', ' ', description).strip()
        docs[path] = {
            "title": title,
            "lib": lib,
            "kind": kind,
            "description": (description or " ").strip(),
            "example": example,
            "details": details,
            "doc_url": doc_url,
        }

    return docs


# ---------------- MAIN ----------------
def build_docs(
        map_file="https://raw.githubusercontent.com/pytgcalls/docsdata/master/map.json",
        config_file="https://raw.githubusercontent.com/pytgcalls/docsdata/master/config.xml",
        output=None,
):
    if output is None:
        repo_root = Path(__file__).resolve().parents[2]
        output = repo_root / "docs.json"

    config_map = parse_config(config_file)
    docs = parse_map(map_file, config_map)
    with open(output, "w", encoding="utf-8") as f:
        json.dump(docs, f, indent=2, ensure_ascii=False)
    print(f"✅ Docs JSON saved to {output}")


if __name__ == "__main__":
    build_docs()

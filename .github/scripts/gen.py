import json
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
            else:
                if result:
                    result[-1]["description"] += (" " + text)
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


def parse_enum_or_type(page, config_map):
    """Extract signature + description + members from enum/type page."""
    details = {}
    sig = page.find("category-title")
    if sig is not None:
        details["signature"] = "".join(sig.itertext()).strip()

    description_parts = []
    members = []
    in_members = False
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

            elif child.tag == "subtext":  # nested description
                desc_txt = "".join(t.strip() for t in child.itertext() if t.strip())
                if current:
                    current["description"] += (" " + desc_txt)
                else:
                    description_parts.append(desc_txt)

            elif child.tag == "config":
                cid = child.attrib["id"]
                text = config_map.get(cid, f"[UNRESOLVED:{cid}]")
                if current:
                    current["description"] += (" " + text.strip())
                else:
                    description_parts.append(text.strip())

            elif child.tag == "text":
                txt = (child.text or "").strip()
                if txt:
                    if current:
                        current["description"] += (" " + txt)
                    else:
                        description_parts.append(txt)

    # walk through top-level subtexts
    for sub in page.findall(".//subtext"):
        for child in sub:
            if child.tag == "pg-title" and "ENUMERATION MEMBERS" in "".join(child.itertext()).upper():
                in_members = True
                continue

            if not in_members:
                if child.tag == "config":
                    cid = child.attrib["id"]
                    description_parts.append(config_map.get(cid, f"[UNRESOLVED:{cid}]"))
                elif child.tag == "text":
                    if child.text:
                        description_parts.append(child.text.strip())
            else:
                if child.tag == "subtext":
                    handle_member_block(child)

    return {
        "signature": details.get("signature"),
        "members": members,
        "description": " ".join(p for p in description_parts if p),
    }



# ---------------- MAP PARSER ----------------
def parse_map(map_source: Union[str, Path], config_map):
    map_text = read_file_or_url(map_source)
    raw_map = json.loads(map_text)
    docs = {}

    for path, page_xml in raw_map.items():
        page = ET.fromstring(page_xml)
        title = page.findtext("h1", "").strip()

        # classify doc kind
        if "/Available Types/" in path:
            kind = "type"
        elif "/Available Enums/" in path:
            kind = "enum"
        else:
            kind = "method"

        # description
        desc_node = page.find("config")
        if desc_node is not None:
            description = config_map.get(desc_node.attrib["id"], "")
        else:
            description = "".join(page.findtext("text", ""))

        # example
        example = {}
        ex_node = page.find("syntax-highlight")
        if ex_node is not None:
            example["language"] = ex_node.attrib.get("language", "python")
            example["code"] = "".join(ex_node.itertext()).strip()

        details = {}

        if kind == "method":
            sig = page.find("category-title")
            if sig is not None:
                details["signature"] = "".join(sig.itertext()).strip()

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
                norm_items = normalize_items(raw_items)
                if norm_items:
                    sections.append({"title": section_title, "items": norm_items})
            if sections:
                details["sections"] = sections

        elif kind in ("enum", "type"):
            parsed = parse_enum_or_type(page, config_map)
            details["signature"] = parsed["signature"]
            details["members"] = parsed["members"]
            if not description:
                description = parsed["description"]

        # doc_url
        doc_path = path.strip("/")
        is_ntgcalls = "ntgcalls" in doc_path.lower()
        if is_ntgcalls and doc_path.startswith("NTgCalls/"):
            doc_path = doc_path[len("NTgCalls/"):]
        elif not is_ntgcalls and doc_path.startswith("PyTgCalls/"):
            doc_path = doc_path[len("PyTgCalls/"):]
        if doc_path.endswith(".xml"):
            doc_path = doc_path[:-4]
        doc_type = "NTgCalls" if is_ntgcalls else "PyTgCalls"
        doc_url = f"https://pytgcalls.github.io/{doc_type}/{doc_path}"

        docs[path] = {
            "title": title,
            "lib": doc_type,
            "kind": kind,
            "description": description,
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

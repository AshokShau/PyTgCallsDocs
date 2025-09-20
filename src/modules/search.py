import json
from dataclasses import dataclass
from typing import List, Optional, Dict


@dataclass
class Member:
    name: str
    type: Optional[str] = None
    description: str = ""
    source_config: Optional[str] = None
    value: Optional[str] = None


@dataclass
class Property:
    name: str
    type: Optional[str] = None
    description: str = ""
    source_config: Optional[str] = None
    value: Optional[str] = None


@dataclass
class Section:
    title: str
    items: List[Dict]


@dataclass
class Details:
    signature: Optional[str] = None
    sections: Optional[List[Section]] = None  # for methods
    members: Optional[List[Member]] = None  # for enums
    properties: Optional[List[Property]] = None  # for types


@dataclass
class DocEntry:
    title: str
    lib: str
    kind: str  # "method" | "enum" | "type"
    description: str
    example: Dict
    details: Details
    doc_url: str


class DocSearch:
    def __init__(self, docs_path: str):
        with open(docs_path, "r", encoding="utf-8") as f:
            raw = json.load(f)

        self.entries: List[DocEntry] = []
        for _, v in raw.items():
            members = None
            if v["details"].get("members"):
                members = [Member(**m) for m in v["details"]["members"]]

            properties = None
            if v["details"].get("properties"):
                properties = [Property(**p) for p in v["details"]["properties"]]

            sections = None
            if v["details"].get("sections"):
                sections = [Section(**s) for s in v["details"]["sections"]]

            details = Details(
                signature=v["details"].get("signature"),
                sections=sections,
                members=members,
                properties=properties,
            )
            self.entries.append(
                DocEntry(
                    title=v["title"],
                    lib=v["lib"],
                    kind=v["kind"],
                    description=v["description"],
                    example=v["example"],
                    details=details,
                    doc_url=v["doc_url"],
                )
            )

    def search(self, query: str, limit: int = 5) -> List[DocEntry]:
        """Smart search across title, description, signature, lib, parameters, members, and properties."""
        q = query.lower()
        scored: List[tuple[int, DocEntry]] = []

        for e in self.entries:
            score = 0

            # title / signature
            if q in e.title.lower():
                score += 10
            if e.details.signature and q in e.details.signature.lower():
                score += 9

            # lib
            if q in e.lib.lower():
                score += 7

            # description
            if q in e.description.lower():
                score += 5

            # sections (PARAMETERS, RAISES)
            if e.details.sections:
                for s in e.details.sections:
                    if q in s.title.lower():
                        score += 4
                    for it in s.items:
                        if q in (it.get("name") or "").lower():
                            score += 4
                        if q in (it.get("type") or "").lower():
                            score += 3
                        if q in (it.get("description") or "").lower():
                            score += 2

            # members (for enums)
            if e.details.members:
                for m in e.details.members:
                    if q in (m.name or "").lower():
                        score += 4
                    if q in (m.value or "").lower():
                        score += 3
                    if q in (m.description or "").lower():
                        score += 2

            # properties (for types)
            if e.details.properties:
                for p in e.details.properties:
                    if q in (p.name or "").lower():
                        score += 4
                    if q in (p.type or "").lower():
                        score += 3
                    if q in (p.description or "").lower():
                        score += 2

            if score > 0:
                scored.append((score, e))

        # sort by score (desc), then shorter title
        scored.sort(key=lambda x: (-x[0], len(x[1].title)))
        return [e for _, e in scored[:limit]]


if __name__ == "__main__":
    import os

    _docs_path = os.path.join(os.path.dirname(os.path.dirname(os.path.dirname(__file__))), "docs.json")
    searcher = DocSearch(_docs_path)

    q = input("🔍 Enter search query: ").strip()
    if results := searcher.search(q, limit=2):
        for r in results:
            print(f"\n📌 {r.title} ({r.kind}) — {r.lib}")
            if r.details.signature:
                print("   Signature:", r.details.signature)
            if r.description:
                print("   Desc:", r.description)
            if r.example:
                print("   Example:", r.example)

            # method sections
            if r.details.sections:
                for s in r.details.sections:
                    print(f"   ▸ {s.title}")
                    for it in s.items:
                        nm = it.get("name") or ""
                        tp = it.get("type") or ""
                        ds = (it.get("description") or "").strip()

                        if s.title.upper() == "RAISES":
                            for line in ds.split("\n"):
                                if line := line.strip():
                                    print(f"      - {line}")
                        else:
                            param_line = f"      {nm}" if nm else "      -"
                            if tp:
                                param_line += f": {tp}"
                            if ds:
                                param_line += f"  # {ds}"
                            print(param_line)

            # enum/type members
            if r.details.members:
                print("   Members:")
                for m in r.details.members:
                    member_line = f"      {m.name}"
                    if m.value:
                        member_line += f" = {m.value}"
                    if m.description:
                        member_line += f" :: {m.description}"
                    print(member_line)

            # type properties
            if r.details.properties:
                print("   Properties:")
                for p in r.details.properties:
                    prop_line = f"      {p.name}"
                    if p.type:
                        prop_line += f" -> {p.type}"
                    if p.description:
                        prop_line += f" :: {p.description}"
                    print(prop_line)

            print("   Doc URL:", r.doc_url)

    else:
        print("❌ No matches found.")

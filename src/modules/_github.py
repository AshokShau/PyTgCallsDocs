import re
import aiohttp

async def _fetch_issue(session, repo, number):
    url = f"https://api.github.com/repos/pytgcalls/{repo}/issues/{number}"
    async with session.get(url) as r:
        if r.status != 200:
            return None
        data = await r.json()
        kind = "PR" if "pull_request" in data else "Issue"
        return {
            "repo": repo,
            "number": number,
            "title": data.get("title"),
            "url": data.get("html_url"),
            "state": data.get("state"),
            "type": kind,
        }

async def search_github_refs(query: str):
    m = re.search(r"^(nt)?#(\d+)$", query, re.IGNORECASE)
    if not m:
        return []

    number = int(m.group(2))
    repos = ["ntgcalls"] if m.group(1) else ["pytgcalls", "ntgcalls"]

    results = []
    async with aiohttp.ClientSession() as session:
        for repo in repos:
            issue = await _fetch_issue(session, repo, number)
            if issue:
                results.append(issue)
    return results

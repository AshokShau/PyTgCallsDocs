import json
import os
import re
from typing import List, Dict, Optional

from pydantic import BaseModel, Field


class SearchResult(BaseModel):
    path: str
    title: str
    score: float
    preview: str = ""

class ParameterInfo(BaseModel):
    name: str
    type: str
    description: str = ""

class PathFullInfo(BaseModel):
    path: str
    title: str
    description: str = ""
    details: str = ""
    parameters: List[ParameterInfo] = Field(default_factory=list)
    examples: List[str] = Field(default_factory=list)

class Search:
    def __init__(self, base_path: str):
        self.base_path = base_path
        self.docs_map = self._load_map()

    def _load_map(self) -> Dict[str, str]:
        """Load the documentation map from map.json."""
        map_path = os.path.join(self.base_path, 'map.json')
        with open(map_path, 'r', encoding='utf-8') as f:
            docs_map = json.load(f)

        # Normalize paths to be consistent
        return {path.lstrip('/'): content for path, content in docs_map.items()}

    @staticmethod
    def _extract_title(content: str) -> str:
        """Extract title from XML content."""
        match = re.search(r'<h1>(.*?)</h1>', content, re.IGNORECASE)
        return match.group(1).strip() if match else ""

    @staticmethod
    def _extract_preview(content: str, query_terms: List[str]) -> str:
        """Extract a preview snippet from content with highlighted query terms."""
        # Remove HTML tags and normalize whitespace
        text = re.sub(r'<[^>]*>', ' ', content)
        text = ' '.join(text.split())

        # Find the best match position
        best_pos = -1
        best_score = 0

        for term in query_terms:
            pos = text.lower().find(term.lower())
            if pos != -1:
                score = len(text) - pos  # Prefer matches closer to the start
                if score > best_score:
                    best_score = score
                    best_pos = pos

        # Extract context around the best match
        if best_pos != -1:
            start = max(0, best_pos - 100)
            end = min(len(text), best_pos + 100)
            preview = text[start:end]
            if start > 0:
                preview = '...' + preview
            if end < len(text):
                preview = preview + '...'

            # Highlight query terms
            for term in query_terms:
                preview = re.sub(
                    f'({re.escape(term)})',
                    r'**\1**',
                    preview,
                    flags=re.IGNORECASE
                )
            return preview

        return text[:200] + '...' if len(text) > 200 else text

    def _calculate_score(self, text: str, query_terms: List[str], is_method: bool = False) -> float:
        """Calculate a relevance score for the given text and query terms."""
        text_lower = text.lower()
        score = 0

        # Check for exact match in title
        title = self._extract_title(text)
        if title:
            title_lower = title.lower()
            if all(term in title_lower for term in query_terms):
                score = 90
            elif any(term in title_lower for term in query_terms):
                score = 70

        # Check content matches
        if score == 0:
            if all(term in text_lower for term in query_terms):
                score = 60
            elif any(term in text_lower for term in query_terms):
                score = 40

        # Bonus for method files
        if is_method and score > 0:
            score = min(100, score + 10)

        return score

    @staticmethod
    def _extract_examples(content: str) -> List[str]:
        """Extract and format code examples from content."""
        examples = []
        example_blocks = re.findall(r'<syntax-highlight[^>]*>(.*?)</syntax-highlight>', content, re.DOTALL)
        
        for block in example_blocks:
            if not block.strip():
                continue
                
            # Split into lines and clean up
            lines = [line.rstrip() for line in block.split('\n')]
            
            # Remove leading/trailing empty lines
            while lines and not lines[0].strip():
                lines.pop(0)
            while lines and not lines[-1].strip():
                lines.pop()
                
            if not lines:
                continue
                
            # Find minimum indentation (skip empty lines)
            min_indent = min(
                len(line) - len(line.lstrip())
                for line in lines
                if line.strip()
            )
            
            # Apply indentation and rejoin
            formatted_lines = []
            for line in lines:
                if line.strip():
                    # Preserve relative indentation
                    formatted_lines.append(line[min_indent:])
                else:
                    # Keep empty lines as is (but trimmed)
                    formatted_lines.append('')
            
            # Join with proper newlines and clean up
            example = '\n'.join(formatted_lines)
            example = re.sub(r'\n{3,}', '\n\n', example)  # Normalize multiple newlines
            
            if example.strip():
                examples.append(example)
                
        return examples

    def _load_config(self):
        """Load the config file with parameter definitions."""
        config_path = os.path.join(self.base_path, 'config.xml')
        if not os.path.exists(config_path):
            return {}
            
        with open(config_path, 'r', encoding='utf-8') as f:
            content = f.read()
            
        config = {}
        # Extract all config options
        options = re.findall(r'<option id="([^"]+)">(.*?)</option>', content, re.DOTALL)
        
        for option_id, option_content in options:
            # Extract parameter name and type
            param_match = re.search(
                r'<category-title>.*?<ref[^>]*><sb>([^<]+)</sb></ref>\s*:?\s*<shi>([^<]+)</shi>',
                option_content,
                re.DOTALL
            )
            
            if param_match:
                param_name = param_match.group(1)
                param_type = param_match.group(2)
                
                # Extract description
                desc_match = re.search(
                    r'<config id="([^"]+)"',
                    option_content
                )
                description = ""
                if desc_match:
                    desc_id = desc_match.group(1)
                    desc_content = re.search(
                        f'<option id="{re.escape(desc_id)}">.*?<text>(.*?)</text>',
                        content,
                        re.DOTALL
                    )
                    if desc_content:
                        description = re.sub(r'<[^>]*>', ' ', desc_content.group(1)).strip()
                
                config[option_id] = {
                    'name': param_name,
                    'type': param_type,
                    'description': description
                }
        
        return config

    def search(self, query: str, limit: int = 10) -> List[SearchResult]:
        """Search through documentation for the given query."""
        query = query.strip().lower()
        if not query:
            return []

        query_terms = [term for term in query.split() if term]
        results = []

        for path, content in self.docs_map.items():
            # Skip non-XML files
            if not path.lower().endswith('.xml'):
                continue

            # Check if this is a method file
            is_method = any(x in path.lower() for x in ['basic method', 'stream method'])

            # Calculate score
            score = self._calculate_score(content, query_terms, is_method)

            if score > 0:
                title = self._extract_title(content) or os.path.splitext(os.path.basename(path))[0]
                preview = self._extract_preview(content, query_terms)

                results.append(SearchResult(
                    path=path,
                    title=title,
                    score=score,
                    preview=preview
                ))

        # Sort by score (highest first) and limit results
        results.sort(key=lambda x: (-x.score, x.path))
        return results[:limit]

    def get_doc_content(self, path: str) -> Optional[str]:
        """Get the full content of a documentation file."""
        return self.docs_map.get(path.lstrip('/'))

    async def get_path_full_info(self, path: str) -> PathFullInfo:
        """Get the full information of a documentation file."""
        # Done ask me how this works :/
        content = self.get_doc_content(path)
        if not content:
            return PathFullInfo(path=path, title=path.split('/')[-1])
            
        # Load config if not already loaded
        if not hasattr(self, '_config'):
            self._config = self._load_config()
            
        title = self._extract_title(content)
        if not title:
            title = path.split('/')[-1].replace('.xml', '').replace('_', ' ').title()

        # Extract description from text node after the title
        description = ""
        desc_match = re.search(r'<h1>.*?</h1>\s*<text>(.*?)</text>', content, re.DOTALL)
        if not desc_match:
            # Try alternative description location
            desc_match = re.search(r'<text>This method (.*?)</text>', content, re.DOTALL)
        if desc_match:
            description = re.sub(r'<[^>]*>', ' ', desc_match.group(1)).strip()
            description = ' '.join(description.split())  # Normalize whitespace
            description = description[0].upper() + description[1:]  # Capitalize first letter

        # Extract details from subtext
        details = ""
        details_match = re.search(r'<subtext>\s*<text>(.*?)</text>', content, re.DOTALL)
        if details_match:
            details = re.sub(r'<[^>]*>', ' ', details_match.group(1))
            details = ' '.join(details.split())  # Normalize whitespace

        # Extract parameters
        parameters = []
        
        # Look for parameters section
        param_section = re.search(r'<pg-title>PARAMETERS</pg-title>(.*?)(?=<pg-title>|$)', content, re.DOTALL | re.IGNORECASE)
        if param_section:
            # Handle config-based parameters first
            config_matches = list(re.finditer(r'<config id="([^"]+)"', param_section.group(1)))
            
            for match in config_matches:
                config_id = match.group(1)
                # Skip certain configs that aren't parameters
                if any(skip in config_id for skip in ['CALL_CONFIG_DESC', 'EXCEPTIONS']):
                    continue
                    
                # Get the next sibling element after the config
                next_part = param_section.group(1)[match.end():match.end()+200]  # Look ahead 200 chars
                
                # Check if this is a parameter with a category-title following it
                param_match = re.search(r'<category-title>(.*?)</category-title>', next_part, re.DOTALL)
                if param_match:
                    # This is a parameter with a custom definition
                    param_text = param_match.group(1)
                    # Extract parameter name and type
                    name_type_match = re.search(r'<ref[^>]*>([^<]+)</ref>\s*:\s*<shi>([^<]+)</shi>', param_text)
                    if name_type_match:
                        param_name = name_type_match.group(1).strip()
                        param_type = name_type_match.group(2).strip()
                        
                        # Look for parameter description
                        desc_start = match.end() + param_match.end()
                        desc_end = param_section.group(1).find('<category-title>', desc_start)
                        if desc_end == -1:
                            desc_end = len(param_section.group(1))
                        
                        param_desc = param_section.group(1)[desc_start:desc_end]
                        # Clean up the description
                        param_desc = re.sub(r'<[^>]*>', ' ', param_desc).strip()
                        param_desc = ' '.join(param_desc.split())  # Normalize whitespace
                        
                        parameters.append(ParameterInfo(
                            name=param_name,
                            type=param_type,
                            description=param_desc
                        ))
                
                # Handle simple config parameters (eg: ARG_CHAT_ID)
                elif config_id.startswith('ARG_') and not any(p.name == 'chat_id' for p in parameters):
                    parameters.append(ParameterInfo(
                        name='chat_id',
                        type='int',
                        description='Unique identifier of a chat'
                    ))
            
            # Also look for direct parameter definitions that might have been missed
            direct_params = re.finditer(
                r'<category-title>\s*<ref[^>]*>([^<]+)</ref>\s*:\s*<shi>([^<]+)</shi>',
                param_section.group(1)
            )
            
            for match in direct_params:
                param_name = match.group(1).strip()
                param_type = match.group(2).strip()
                
                # Skip if we already have this parameter
                if any(p.name == param_name for p in parameters):
                    continue
                
                # Look for parameter description
                param_desc = ""
                desc_match = re.search(
                    rf'<category-title>\s*<ref[^>]*>{re.escape(param_name)}</ref>\s*:\s*<shi>[^<]*</shi>.*?<subtext>\s*<text>(.*?)</text>',
                    param_section.group(1),
                    re.DOTALL
                )
                if desc_match:
                    param_desc = re.sub(r'<[^>]*>', ' ', desc_match.group(1)).strip()
                
                parameters.append(ParameterInfo(
                    name=param_name,
                    type=param_type,
                    description=param_desc
                ))
        
        # Extract examples
        examples = self._extract_examples(content)
        
        return PathFullInfo(
            path=path,
            title=title,
            description=description,
            details=details,
            parameters=parameters,
            examples=examples
        )

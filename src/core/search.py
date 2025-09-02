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
    return_type: str = ""

class Search:
    def __init__(self, base_path: str):
        self.base_path = base_path
        self.docs_map = self._load_map()
        self._config = self._load_config()

    def _load_map(self) -> Dict[str, str]:
        """Load the documentation map from map.json."""
        map_path = os.path.join(self.base_path, 'map.json')
        with open(map_path, 'r', encoding='utf-8') as f:
            docs_map = json.load(f)

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

        best_pos = -1
        best_score = 0

        for term in query_terms:
            pos = text.lower().find(term.lower())
            if pos != -1:
                score = len(text) - pos
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
        title = self._extract_title(text)
        title_lower = title.lower() if title else ""

        # Check for exact matches first
        exact_query = ' '.join(query_terms).lower()
        
        # Boost for exact match in title
        if title and exact_query in title_lower:
            score += 100
        
        # Check for method signature match (e.g., "mute(chat_id: int)")
        method_match = re.search(r'<ref[^>]*>([^<]+)</ref>\s*\(([^)]*)\)', text, re.IGNORECASE)
        if method_match:
            method_name = method_match.group(1).lower()
            method_params = method_match.group(2).lower()
            
            # Exact method name match
            if exact_query == method_name:
                score += 95
            # Partial method name match
            elif all(term in method_name for term in query_terms):
                score += 80
            # Match in method signature
            elif any(term in method_params for term in query_terms):
                score += 60

        # Check for parameter matches
        param_matches = re.findall(r'<ref[^>]*>([^<]+)</ref>\s*:\s*([^<,]+)', text, re.IGNORECASE)
        for param_name, param_type in param_matches:
            param_name = param_name.strip().lower()
            param_type = param_type.strip().lower()
            
            # Exact parameter name match
            if exact_query == param_name:
                score += 90
            # Partial parameter name match
            elif all(term in param_name for term in query_terms):
                score += 70
            # Type match
            elif exact_query in param_type:
                score += 60

        # Check for matches in title
        if not score and title:
            if all(term in title_lower for term in query_terms):
                score = 50
            elif any(term in title_lower for term in query_terms):
                score = 30

        # Check for matches in content
        if not score:
            if all(term in text_lower for term in query_terms):
                score = 40
            elif any(term in text_lower for term in query_terms):
                score = 20

        # Boost for method files
        if is_method and score > 0:
            score += 10

        # Boost for exact matches in code blocks
        code_blocks = re.findall(r'<syntax-highlight[^>]*>(.*?)</syntax-highlight>', text, re.DOTALL)
        for block in code_blocks:
            if exact_query in block.lower():
                score += 20
                break

        return min(100, score)

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
        
        # Extract all options
        options = re.findall(r'<option id="([^"]+)">(.*?)</option>', content, re.DOTALL)
        
        for option_id, option_content in options:
            # Skip description-only options (handled separately)
            if option_id.endswith('_DESC'):
                continue
                
            # This is a parameter definition
            param_info = {'id': option_id}
            
            # Extract parameter name and type from category-title
            # Try pattern for <ref><sb>param_name</sb></ref> format
            param_match = re.search(
                r'<category-title>.*?<ref><sb>([^<]+)</sb></ref>\s*:\s*(.*?)(?:<|$)',
                option_content,
                re.DOTALL
            )
            
            # If first pattern didn't match, try pattern for <ref>param_name</ref> format
            if not param_match:
                param_match = re.search(
                    r'<category-title>.*?<ref>([^<]+)</ref>\s*:\s*(.*?)(?:<|$)',
                    option_content,
                    re.DOTALL
                )
            
            if param_match:
                param_name = param_match.group(1).strip()
                param_type = param_match.group(2).strip()
                
                # Clean up the type (remove HTML tags but preserve content)
                param_type = re.sub(r'<[^>]+>', '', param_type)  # Remove all HTML tags
                param_type = re.sub(r'\s+', ' ', param_type).strip()  # Normalize whitespace
                
                # Extract description from subtext if available
                desc_match = re.search(
                    r'<subtext>\s*<text>(.*?)</text>',
                    option_content,
                    re.DOTALL
                )
                
                param_info.update({
                    'name': param_name,
                    'type': param_type,
                    'description': re.sub(r'<[^>]*>', ' ', desc_match.group(1)).strip() if desc_match else ''
                })
                
                # Handle description references (e.g., CHAT_ID_DESC)
                desc_ref_match = re.search(
                    r'<config id="([^"]+_DESC)"',
                    option_content
                )
                
                if desc_ref_match:
                    desc_ref_id = desc_ref_match.group(1)
                    desc_ref_content = re.search(
                        f'<option id="{re.escape(desc_ref_id)}">.*?<text>(.*?)</text>',
                        content,
                        re.DOTALL
                    )
                    
                    if desc_ref_content:
                        param_info['description'] = re.sub(r'<[^>]*>', ' ', desc_ref_content.group(1)).strip()
                
                config[option_id] = param_info
        
        return config

    def search(self, query: str, limit: int = 20) -> List[SearchResult]:
        """Search through documentation for the given query."""
        query = query.strip().lower()
        if not query:
            return []

        query_terms = [term for term in query.split() if len(term) > 1]
        results = []

        for path, content in self.docs_map.items():
            # Skip non-XML files
            if not path.lower().endswith('.xml'):
                continue

            # Check if this is a method file
            is_method = any(x in path.lower() for x in ['basic method', 'stream method', 'advanced method'])
            
            # Check if this is NTgCalls or PyTgCalls
            is_ntgcalls = 'ntgcalls' in path.lower()
            is_pytgcalls = 'pytgcalls' in path.lower()

            # Calculate score
            score = self._calculate_score(content, query_terms, is_method)

            if score > 0:
                title = self._extract_title(content) or os.path.splitext(os.path.basename(path))[0]
                preview = self._extract_preview(content, query_terms)

                if is_ntgcalls:
                    title = f"[NTgCalls] {title}"
                elif is_pytgcalls:
                    title = f"[PyTgCalls] {title}"

                results.append(SearchResult(
                    path=path,
                    title=title,
                    score=score,
                    preview=preview
                ))

        # Sort by score (highest first) and then by path
        results.sort(key=lambda x: (-x.score, x.path))
        return results[:limit]

    def get_doc_content(self, path: str) -> Optional[str]:
        """Get the full content of a documentation file."""
        return self.docs_map.get(path.lstrip('/'))

    async def get_path_full_info(self, path: str) -> PathFullInfo:
        """Get the full information of a documentation file."""
        full_path = os.path.join(self.base_path, path)
        
        if not os.path.exists(full_path):
            return PathFullInfo(path=path, title="Not Found", description="The requested documentation was not found.")
            
        with open(full_path, 'r', encoding='utf-8') as f:
            content = f.read()
            
        # Extract title
        title = self._extract_title(content)
        
        # Extract description (first paragraph)
        description = ""
        desc_match = re.search(r'<subtext>\s*<text>([^<]+)', content)
        if desc_match:
            description = desc_match.group(1).strip()
            
        # Extract details from subtext
        details = ""
        details_match = re.search(r'<subtext>\s*<text>(.*?)</text>', content, re.DOTALL)
        if details_match:
            details = re.sub(r'<[^>]*>', ' ', details_match.group(1))
            details = ' '.join(details.split())

        # Extract return type
        return_type = ""
        return_match = re.search(r'<ref[^>]*>.*?</ref>\s*<shi>\s*->\s*(.*?)</shi>', content, re.DOTALL)
        if return_match:
            return_type = return_match.group(1).strip()
            # Clean up the return type
            return_type = re.sub(r'<[^>]*>', '', return_type)
            return_type = ' '.join(return_type.split())

        # Extract parameters
        parameters = self._extract_parameters(content)
        
        # Extract examples
        examples = self._extract_examples(content)
        
        return PathFullInfo(
            path=path,
            title=title,
            description=description,
            details=details,
            parameters=parameters,
            examples=examples,
            return_type=return_type
        )

    def _extract_parameters(self, content: str) -> List[ParameterInfo]:
        """Extract parameters from content, handling both NTgCalls and PyTgCalls formats."""
        parameters = []
        
        # Look for parameters section
        param_section = re.search(r'<pg-title>PARAMETERS</pg-title>(.*?)(?=<pg-title>|$)', content, re.DOTALL | re.IGNORECASE)
        if not param_section:
            return parameters

        config_matches = list(re.finditer(r'<config id="([^"]+)"', param_section.group(1)))
        for match in config_matches:
            config_id = match.group(1)
            if config_id in self._config and not any(skip in config_id for skip in ['EXCEPTIONS']):
                config = self._config[config_id]
                if isinstance(config, dict) and 'name' in config and 'type' in config:
                    param_type = config['type']
                    if not any(p.name == config['name'] for p in parameters):
                        parameters.append(ParameterInfo(
                            name=config['name'],
                            type=param_type,
                            description=config.get('description', '')
                        ))

        param_blocks = re.findall(
            r'(?:<category-title>(.*?)</category-title>|<config id="([^"]+)"\s*/>)\s*(<subtext>.*?</subtext>)?',
            param_section.group(1),
            re.DOTALL
        )
        
        for category_content, config_id, subtext_content in param_blocks:
            # Handle config parameters
            if config_id and config_id in self._config and not any(skip in config_id for skip in ['EXCEPTIONS']):
                config = self._config[config_id]
                if isinstance(config, dict) and 'name' in config and 'type' in config:
                    if not any(p.name == config['name'] for p in parameters):
                        # Get description from subtext if available
                        desc = config.get('description', '')
                        if subtext_content:
                            desc_match = re.search(r'<text>(.*?)</text>', subtext_content, re.DOTALL)
                            if desc_match:
                                desc = re.sub(r'<[^>]*>', ' ', desc_match.group(1)).strip()
                        
                        parameters.append(ParameterInfo(
                            name=config['name'],
                            type=config['type'],
                            description=desc
                        ))
                continue
                
            # Skip if no category content (was a config parameter)
            if not category_content:
                continue
                
            # Extract parameter name and type - handle multiple formats
            param_match = None
            
            # Try different patterns in order of specificity
            patterns = [
                # Pattern for complex type definitions with refs
                r'<ref[^>]*>([^<]+)</ref>\s*:\s*((?:<[^>]+>|\[|\]|\w|\s|,|\.|\|)+)',
                # Pattern for simple type definitions with refs
                r'<ref[^>]*>([^<]+)</ref>\s*<ref[^>]*>([^<]+)</ref>',
                # Fallback pattern
                r'<ref[^>]*>([^<]+)</ref>\s*:\s*([^<,]+)'
            ]
            
            for pattern in patterns:
                param_match = re.search(pattern, category_content, re.DOTALL)
                if param_match:
                    break
            
            if param_match:
                param_name = param_match.group(1).strip()
                param_type = param_match.group(2).strip()
                
                # Clean up the type (remove HTML tags but preserve content)
                param_type = re.sub(r'<[^>]+>', '', param_type)
                param_type = ' '.join(param_type.split())
                
                # Skip if we already have this parameter
                if any(p.name == param_name for p in parameters):
                    continue
                
                # Find the description in the subtext
                param_desc = ""
                if subtext_content:
                    # Look for description in subtext
                    desc_match = re.search(
                        r'<text>(.*?)</text>',
                        subtext_content,
                        re.DOTALL
                    )
                    if desc_match:
                        param_desc = re.sub(r'<[^>]*>', ' ', desc_match.group(1)).strip()
                    
                    # If no description in subtext, check for config description
                    if not param_desc and '<config id=' in subtext_content:
                        config_match = re.search(r'<config id="([^"]+)"', subtext_content)
                        if config_match and config_match.group(1) in self._config:
                            config = self._config[config_match.group(1)]
                            if isinstance(config, dict) and 'description' in config:
                                param_desc = config['description']
                
                parameters.append(ParameterInfo(
                    name=param_name,
                    type=param_type,
                    description=param_desc
                ))
        
        return parameters

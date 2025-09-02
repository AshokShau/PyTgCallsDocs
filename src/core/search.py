import json
import os
import re
from typing import List, Dict, Optional
from pydantic import BaseModel, Field
from xml.etree import ElementTree as ET


class SearchResult(BaseModel):
    path: str
    title: str
    score: float
    preview: str = ""


class ParameterInfo(BaseModel):
    name: str
    type: str
    description: str = ""


class EnumMemberInfo(BaseModel):
    name: str
    value: str = ""
    description: str = ""


class MethodInfo(BaseModel):
    name: str
    signature: str = ""
    return_type: str = ""
    description: str = ""


class PropertyInfo(BaseModel):
    """Information about a property."""
    name: str
    return_type: str = ""
    description: str = ""


class PathFullInfo(BaseModel):
    """Full information about a documentation path."""
    path: str
    title: str
    class_info: str = ""
    description: str = ""
    details: str = ""
    parameters: List[ParameterInfo] = Field(default_factory=list)
    properties: List[PropertyInfo] = Field(default_factory=list)
    enum_members: List[EnumMemberInfo] = Field(default_factory=list)
    methods: List[MethodInfo] = Field(default_factory=list)
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
    def _extract_class_info(content: str) -> str:
        class_match = re.search(
            r'<category-title[^>]*>\s*<shi>class</shi>\s*<ref[^>]*>.*?<sb>(.*?)</sb>',
            content,
            re.DOTALL
        )
        
        if class_match:
            return class_match.group(1).strip()
            
        # If not a class, try to find method signature
        method_match = re.search(
            r'<category-title[^>]*>\s*<ref[^>]*>.*?<sb>(.*?)</sb></ref>\s*(\([^)]*\))?\s*(<shi>->\s*(.*?)</shi>)?',
            content,
            re.DOTALL
        )
        
        if method_match:
            return method_match.group(1).strip()
            
        return ""

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

    def _load_config(self) -> dict:
        """Load the config file with all parameter definitions and descriptions."""
        config_path = os.path.join(self.base_path, 'docsdata', 'config.xml')
        if not os.path.exists(config_path):
            return {}
            
        with open(config_path, 'r', encoding='utf-8') as f:
            content = f.read()
            
        # First pass: load all config options
        config = {}
        option_pattern = r'<option id="([^"]+)">(.*?)</option>'
        
        for match in re.finditer(option_pattern, content, re.DOTALL):
            option_id = match.group(1)
            option_content = match.group(2)
            
            config[option_id] = {}
            
            # Parse category-title if exists
            category_title = re.search(r'<category-title>(.*?)</category-title>', option_content, re.DOTALL)
            if category_title:
                # Clean up the category title to handle both -> and : as type separators
                title = category_title.group(1).strip()
                # Replace any HTML formatting that might interfere with type extraction
                title = re.sub(r'<[^>]+>', ' ', title)  # Replace HTML tags with spaces
                title = ' '.join(title.split())  # Normalize whitespace
                config[option_id]['category-title'] = title
                
            # Parse subtext and text
            subtext = re.search(r'<subtext>(.*?)</subtext>', option_content, re.DOTALL)
            if subtext:
                text_match = re.search(r'<text>(.*?)</text>', subtext.group(1), re.DOTALL)
                if text_match:
                    config[option_id]['text'] = text_match.group(1).strip()
                else:
                    # Handle case where subtext doesn't contain text tags
                    subtext_content = re.sub(r'<[^>]+>', ' ', subtext.group(1))
                    subtext_content = ' '.join(subtext_content.split()).strip()
                    if subtext_content:
                        config[option_id]['subtext'] = subtext_content
            
            # Parse nested config references
            config_ref = re.search(r'<config id="([^"]+)"', option_content)
            if config_ref:
                config[option_id]['config'] = {'id': config_ref.group(1)}
        
        # Second pass: resolve all nested config references
        for option_id in list(config.keys()):  # Use list to avoid modifying dict during iteration
            if option_id in config:  # Check if still exists (might have been merged)
                self._resolve_nested_configs(option_id, config, set())
        
        return config
    
    def _resolve_nested_configs(self, config_id: str, config: dict, visited: set) -> None:
        """Recursively resolve nested config references."""
        if config_id in visited:
            return
            
        visited.add(config_id)
        
        if config_id not in config:
            return
            
        option_data = config[config_id]
        
        # If this option has a config reference, resolve it
        if 'config' in option_data and 'id' in option_data['config']:
            nested_id = option_data['config']['id']
            
            # Make sure the nested config is resolved first
            if nested_id in config and nested_id not in visited:
                self._resolve_nested_configs(nested_id, config, visited)
            
            # Copy fields from the nested config
            if nested_id in config:
                nested_config = config[nested_id]
                
                # Copy text/subtext if not already present
                if 'text' not in option_data and 'text' in nested_config:
                    option_data['text'] = nested_config['text']
                elif 'subtext' not in option_data and 'subtext' in nested_config:
                    option_data['subtext'] = nested_config['subtext']
                
                # Copy category-title if we don't have a description yet
                if 'category-title' not in option_data and 'category-title' in nested_config:
                    option_data['category-title'] = nested_config['category-title']

    def _resolve_config_placeholders(self, text: str) -> str:
        """Replace config placeholders in the text with their corresponding values.
        
        Args:
            text: The text containing config placeholders
            
        Returns:
            Text with config placeholders replaced by their values
        """
        if not text or not hasattr(self, '_config'):
            return text
            
        def replace_match(match):
            config_id = match.group(1)
            # Get the config value, or return a placeholder if not found
            return self._config.get(config_id, f"[Config not found: {config_id}]")
            
        # Replace <config id="ID"/> with the corresponding config value
        text = re.sub(r'<config\s+id="([^"]+)"\s*/>', replace_match, text)
        
        # Also handle any remaining config references in the text
        while '<config ' in text:
            text = re.sub(r'<config\s+id="([^"]+)"\s*/>', replace_match, text)
            
        return text.strip()

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
        title = self._extract_title(content) or path

        # Extract class information if available
        class_info = self._extract_class_info(content)

        # Extract description and details
        description = ""
        details = ""

        # First try to get the full subtext content
        full_subtext_match = re.search(r'<subtext>\s*<text>(.*?)</text>', content, re.DOTALL)
        if full_subtext_match:
            full_text = full_subtext_match.group(1).strip()

            # Get the first sentence/paragraph as description
            first_paragraph_match = re.search(r'^([^<\n]+)', full_text)
            if first_paragraph_match:
                description = first_paragraph_match.group(1).strip()

                # Remove any HTML tags from description
                description = re.sub(r'<[^>]*>', ' ', description)
                description = ' '.join(description.split())

                # Get the rest as details, excluding the first paragraph we already have
                details_start = first_paragraph_match.end()
                details = full_text[details_start:].strip()

                # Clean up details
                if details:
                    details = re.sub(r'<[^>]*>', ' ', details)
                    details = ' '.join(details.split())

                    # If details starts with the same text as description, remove it
                    if details.startswith(description):
                        details = details[len(description):].strip()

        # Extract return type
        return_type = ""
        return_match = re.search(r'<ref[^>]*>.*?</ref>\s*<shi>-></shi>\s*<docs-ref[^>]*>([^<]+)</docs-ref>', content, re.DOTALL)
        if return_match:
            return_type = return_match.group(1).strip()
            # Clean up the return type
            return_type = re.sub(r'<[^>]+>', '', return_type)
            return_type = ' '.join(return_type.split())

        # Extract parameters
        parameters = self._extract_parameters(content)

        # Extract methods
        methods = self._extract_methods(content)

        # Extract examples
        examples = self._extract_examples(content)

        properties = self._extract_properties(content)
        enum_members = self._parse_enum_members(content)
        return PathFullInfo(
            path=path,
            title=title,
            class_info=class_info,
            description=description,
            details=details,
            parameters=parameters,
            properties=properties,
            methods=methods,
            enum_members=enum_members,
            examples=examples,
            return_type=return_type
        )

    def _extract_properties(self, content: str) -> List[PropertyInfo]:
        """Extract properties from the XML content."""
        properties = []
        
        # Look for properties section
        prop_section = re.search(
            r'<pg-title>PROPERTIES</pg-title>(.*?)(?=<pg-title>|$)', 
            content, 
            re.DOTALL | re.IGNORECASE
        )

        if not prop_section:
            return properties

        # First, process all standalone config references
        standalone_configs = re.finditer(
            r'<config id="([^"]+)"\s*/>',
            prop_section.group(1)
        )
        
        for config_match in standalone_configs:
            config_id = config_match.group(1)
            if config_id in self._config:
                config = self._config[config_id]
                # Check if this config has a nested config reference
                if 'config' in config and 'id' in config['config']:
                    nested_id = config['config']['id']
                    if nested_id in self._config:
                        # Merge the nested config with the current one
                        nested_config = self._config[nested_id]
                        config = {**nested_config, **config}
                
                if 'category-title' in config:
                    # Clean up the title
                    title = re.sub(r'<[^>]+>', ' ', config['category-title'])
                    title = ' '.join(title.split())
                    
                    # Get description from text or subtext
                    description = ''
                    if 'text' in config:
                        description = config['text']
                    elif 'subtext' in config:
                        description = config['subtext']
                    
                    # Extract property name from <ref><sb>property_name</sb></ref> format
                    prop_match = re.search(r'<ref><sb>([^<]+)</sb>', config.get('category-title', ''))
                    if not prop_match:
                        # Fall back to other formats if needed
                        prop_match = re.search(r'<ref>([^<]+)</ref>', config.get('category-title', ''))
                    
                    if prop_match:
                        prop_name = prop_match.group(1).strip()
                        # Try to extract return type if available
                        type_match = re.search(r'<shi>-></shi>\s*<docs-ref[^>]*>([^<]+)</docs-ref>', config.get('category-title', ''))
                        if not type_match:
                            type_match = re.search(r'<shi>->\s*([^<]+)</shi>', config.get('category-title', ''))
                        prop_type = type_match.group(1).strip() if type_match else 'Any'
                        
                        # Add the property if it's not already in the list
                        if not any(p.name == prop_name for p in properties):
                            properties.append(PropertyInfo(
                                name=prop_name,
                                return_type=prop_type,
                                description=description
                            ))
            
        # Then process all regular property definitions
        prop_blocks = re.finditer(
            r'<category-title>(.*?)</category-title>\s*<subtext>(.*?)</subtext>',
            prop_section.group(1),
            re.DOTALL
        )

        for match in prop_blocks:
            title = match.group(1).strip()
            subtext = match.group(2).strip()
            
            # Skip if this is just a config ID without a property
            if not title.strip():
                continue
                
            # Extract property name and type
            prop_match = re.search(
                r'<ref>(?:<sb>)?([^<]+)(?:</sb>)?</ref>\s*<shi>-></shi>\s*(.*?)(?=<|$)',
                title,
                re.DOTALL
            )
            
            if not prop_match:
                continue
                
            prop_name = prop_match.group(1).strip()
            return_type = prop_match.group(2).strip()
            
            # Clean up the return type
            if '<docs-ref' in return_type:
                # Handle simple type references
                ref_match = re.search(r'<docs-ref[^>]*>([^<]+)</docs-ref>', return_type, re.DOTALL)
                if ref_match:
                    return_type = ref_match.group(1).strip()
            else:
                # Clean up any remaining HTML tags
                return_type = re.sub(r'<[^>]+>', '', return_type).strip()
            
            # Extract description
            desc_match = re.search(r'<text>(.*?)</text>', subtext, re.DOTALL)
            description = desc_match.group(1).strip() if desc_match else ''
            
            # Add the property if it's not already in the list
            if not any(p.name == prop_name for p in properties):
                properties.append(PropertyInfo(
                    name=prop_name,
                    return_type=return_type,
                    description=description
                ))
            
        return properties

    def _extract_parameters(self, content: str) -> List[ParameterInfo]:
        """Extract parameters from content, handling both NTgCalls and PyTgCalls formats."""
        parameters = []

        # Look for parameters section
        param_section = re.search(r'<pg-title>PARAMETERS</pg-title>(.*?)(?=<pg-title>|$)', content,
                                  re.DOTALL | re.IGNORECASE)
        if not param_section:
            return parameters

        config_matches = list(re.finditer(
            r'<config id="([^"]+)"',
            param_section.group(1)
        ))
        
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
                            desc_match = re.search(
                                r'<text>(.*?)</text>',
                                subtext_content,
                                re.DOTALL
                            )
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
                    if not param_desc and '<config ' in subtext_content:
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

    @staticmethod
    def _extract_methods(content: str) -> List[MethodInfo]:
        """Extract methods from content."""
        methods = []

        # Look for methods section
        method_section = re.search(r'<pg-title>METHODS</pg-title>(.*?)(?=<pg-title>|$)', content,
                                   re.DOTALL | re.IGNORECASE)
        if not method_section:
            return methods

        # Find all method blocks
        method_blocks = re.finditer(
            r'<category-title>(.*?)</category-title>\s*<subtext>\s*<text>(.*?)</text>',
            method_section.group(1),
            re.DOTALL
        )

        for method_block in method_blocks:
            method_title = method_block.group(1)
            method_desc = method_block.group(2).strip()

            # Extract method name
            method_name_match = re.search(
                r'<ref[^>]*>.*?<sb>(.*?)</sb>',
                method_title
            )
            if not method_name_match:
                continue

            method_name = method_name_match.group(1).strip()

            # Extract method signature (parameters)
            signature_match = re.search(
                r'<sb>.*?</sb>\s*(\([^)]*\))',
                method_title
            )
            method_signature = signature_match.group(1) if signature_match else "()"

            # Extract return type (everything after -> if it exists)
            return_type = ""
            if '->' in method_title:
                # Get everything after ->
                return_part = method_title.split('->', 1)[1]
                # Remove any HTML tags and normalize whitespace
                return_type = re.sub(r'<[^>]+>', '', return_part)
                return_type = ' '.join(return_type.split())

            # Clean up the method signature
            method_signature = re.sub(r'<[^>]+>', '', method_signature)
            method_signature = ' '.join(method_signature.split())

            # Clean up the description
            method_desc = re.sub(r'<[^>]*>', ' ', method_desc)
            method_desc = ' '.join(method_desc.split())

            methods.append(MethodInfo(
                name=method_name,
                signature=method_signature,
                return_type=return_type,
                description=method_desc
            ))

        return methods

    def _parse_enum_members(self, content: str) -> List[EnumMemberInfo]:
        """Parse enum members from the XML content."""
        enum_members = []

        # Look for enumeration members section
        members_section = re.search(
            r'<pg-title>ENUMERATION MEMBERS</pg-title>(.*?)(?=<pg-title>|$)',
            content,
            re.DOTALL | re.IGNORECASE
        )

        if not members_section:
            return enum_members

        # Find all member blocks with optional config references
        member_blocks = re.finditer(
            r'<category-title>(.*?)</category-title>\s*(?:<config id="([^"]+)"\s*/?>)?',
            members_section.group(1),
            re.DOTALL
        )

        for match in member_blocks:
            member_title = match.group(1).strip()
            config_id = match.group(2) if len(match.groups()) > 1 and match.group(2) else None

            # Extract member name and value - handle both formats:
            # 1. <ref><sb>NAME</sb></ref> <shi>=</shi> VALUE
            # 2. <ref>NAME</ref> <shi>=</shi> VALUE
            member_match = re.search(
                r'<ref>(?:<sb>)?([^<]+)(?:</sb>)?</ref>\s*<shi>=</shi>\s*([^<\s]+)',
                member_title
            )

            if not member_match:
                continue

            name = member_match.group(1).strip()
            value = member_match.group(2).strip()

            # Get description from config if available
            description = ""
            if config_id and hasattr(self, '_config') and config_id in self._config:
                config = self._config[config_id]
                if 'text' in config:
                    description = config['text']
                elif 'subtext' in config:
                    description = config['subtext']
                elif 'category-title' in config:
                    # Fallback to category-title if no text/subtext
                    description = config['category-title']

            # Clean up the description
            if description:
                description = re.sub(r'<[^>]*>', '', description).strip()

            # Add the enum member if it's not already in the list
            if not any(m.name == name for m in enum_members):
                enum_members.append(EnumMemberInfo(
                    name=name,
                    value=value,
                    description=description
                ))

        return enum_members

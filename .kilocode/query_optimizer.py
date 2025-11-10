#!/usr/bin/env python3
"""
Query Optimizer for TinyGo Project
Optimizes queries to reduce token usage and prevent context window overflow.
"""

import re
import json
import os
from typing import Dict, List, Optional, Any, Tuple
from context_manager import ContextManager

class QueryOptimizer:
    """Optimizes queries to minimize token usage."""
    
    def __init__(self, config_path: str = ".kilocode/config.json"):
        self.config = self._load_config(config_path)
        self.context_manager = ContextManager(config_path)
        
    def _load_config(self, config_path: str) -> Dict[str, Any]:
        """Load configuration from JSON file."""
        try:
            with open(config_path, 'r', encoding='utf-8') as f:
                return json.load(f)
        except (FileNotFoundError, json.JSONDecodeError):
            return {}
    
    def optimize_search_query(self, query: str, file_pattern: str = "*") -> str:
        """Optimize a search query to reduce token usage."""
        # Remove redundant whitespace
        optimized = re.sub(r'\s+', ' ', query.strip())
        
        # Remove redundant keywords
        optimized = re.sub(r'\b(in|the|a|an|and|or|but|is|are|was|were|be|been|have|has|had|do|does|did|will|would|could|should|may|might|must|shall|can|to|of|for|with|at|by|from|up|about|into|through|during|before|after|above|below|between|among|to|from|up|down|in|out|on|off|over|under|again|further|then|once)\b', '', optimized, flags=re.IGNORECASE)
        
        # Remove duplicate words
        words = optimized.split()
        seen = set()
        unique_words = []
        for word in words:
            if word.lower() not in seen:
                seen.add(word.lower())
                unique_words.append(word)
        
        optimized = ' '.join(unique_words)
        
        # Add specific file pattern if provided
        if file_pattern and file_pattern != "*":
            optimized += f" in {file_pattern}"
        
        return optimized
    
    def chunk_large_request(self, request: str, max_chunk_size: int = None) -> List[str]:
        """Split a large request into smaller chunks."""
        if max_chunk_size is None:
            max_chunk_size = self.context_manager.get_chunk_size()
        
        # Split by sentences first
        sentences = re.split(r'(?<=[.!?])\s+', request)
        
        chunks = []
        current_chunk = ""
        current_size = 0
        
        for sentence in sentences:
            sentence_size = len(sentence.split())
            
            if current_size + sentence_size <= max_chunk_size:
                current_chunk += sentence + " "
                current_size += sentence_size
            else:
                if current_chunk:
                    chunks.append(current_chunk.strip())
                current_chunk = sentence + " "
                current_size = sentence_size
        
        if current_chunk:
            chunks.append(current_chunk.strip())
        
        return chunks
    
    def prioritize_search_terms(self, terms: List[str]) -> List[str]:
        """Prioritize search terms by importance."""
        # Prioritize technical terms over common words
        technical_priority = []
        common_priority = []
        
        common_words = {'the', 'and', 'or', 'but', 'in', 'on', 'at', 'to', 'for', 'of', 'with', 'by', 'from', 'up', 'about', 'into', 'through', 'during', 'before', 'after', 'above', 'below', 'between', 'among', 'again', 'further', 'then', 'once'}
        
        for term in terms:
            if term.lower() not in common_words and len(term) > 2:
                technical_priority.append(term)
            else:
                common_priority.append(term)
        
        # Technical terms first, then common terms
        return technical_priority + common_priority
    
    def create_focused_query(self, file_path: str, search_area: str = "code") -> str:
        """Create a focused query based on file type and search area."""
        file_ext = os.path.splitext(file_path)[1].lower()
        
        if file_ext == '.go':
            if search_area == "code":
                return "func|struct|type|interface|method|variable|constant"
            elif search_area == "imports":
                return "import|package"
            elif search_area == "errors":
                return "error|panic|recover|fail|exception"
        elif file_ext in ['.h', '.hpp', '.c', '.cpp']:
            if search_area == "code":
                return "function|class|struct|typedef|define|macro"
            elif search_area == "headers":
                return "#include|#pragma|extern"
        
        # Default focused query
        return search_area
    
    def estimate_token_usage(self, text: str) -> int:
        """Estimate token usage for a given text."""
        # Rough approximation: 1 token ≈ 4 characters for English text
        return len(text) // 4
    
    def optimize_file_analysis(self, file_path: str) -> Dict[str, Any]:
        """Optimize file analysis parameters."""
        file_size = os.path.getsize(file_path) if os.path.exists(file_path) else 0
        
        optimization = {
            "file_path": file_path,
            "file_size": file_size,
            "should_chunk": self.context_manager.should_chunk_file(file_path),
            "chunk_size": self.context_manager.get_chunk_size(),
            "max_depth": self.context_manager.get_max_recursive_depth(),
            "use_semantic_search": self.context_manager.should_use_semantic_search()
        }
        
        return optimization
    
    def create_analysis_plan(self, target_path: str, analysis_type: str = "general") -> List[str]:
        """Create an optimized analysis plan."""
        plan = []
        
        if analysis_type == "general":
            # Use semantic search first if configured
            if self.context_manager.should_use_semantic_search():
                plan.append("semantic_search")
            
            plan.extend([
                "file_structure_analysis",
                "import_analysis",
                "function_analysis",
                "error_pattern_analysis"
            ])
        
        elif analysis_type == "security":
            plan.extend([
                "vulnerability_scan",
                "input_validation_check",
                "buffer_overflow_check",
                "memory_safety_analysis"
            ])
        
        elif analysis_type == "performance":
            plan.extend([
                "performance_bottleneck_analysis",
                "memory_usage_analysis",
                "algorithm_complexity_check"
            ])
        
        return plan
    
    def validate_query_safety(self, query: str) -> Tuple[bool, str]:
        """Validate query to prevent excessive token usage."""
        estimated_tokens = self.estimate_token_usage(query)
        
        if estimated_tokens > self.context_manager.max_tokens_per_request:
            return False, f"Query too large: {estimated_tokens} tokens > {self.context_manager.max_tokens_per_request}"
        
        # Check for potentially problematic patterns
        dangerous_patterns = [
            r'.*\.go.*',  # All Go files
            r'.*src.*',   # All src directory
            r'.*\.ALL.*', # All files
            r'.*\*.*',    # Wildcard patterns
        ]
        
        for pattern in dangerous_patterns:
            if re.match(pattern, query, re.IGNORECASE):
                return False, f"Potentially dangerous query pattern: {pattern}"
        
        return True, "Query is safe"
    
    def get_optimized_search_strategy(self, search_term: str, directory: str = ".") -> Dict[str, Any]:
        """Get optimized search strategy for given term."""
        validation_result, validation_msg = self.validate_query_safety(search_term)
        
        if not validation_result:
            return {
                "valid": False,
                "error": validation_msg,
                "suggestion": f"Try a more specific search term or use chunking: {self.context_manager.get_chunk_size()}"
            }
        
        strategy = {
            "search_term": search_term,
            "directory": directory,
            "validation": {
                "valid": True,
                "message": validation_msg
            },
            "optimization": {
                "use_semantic_search": self.context_manager.should_use_semantic_search(),
                "chunk_if_needed": self.context_manager.should_chunk_file(directory),
                "prioritized_terms": self.prioritize_search_terms(search_term.split())
            }
        }
        
        return strategy

def main():
    """Main function for testing."""
    optimizer = QueryOptimizer()
    
    # Test query optimization
    test_query = "find all functions that handle errors and exceptions in the src directory"
    optimized = optimizer.optimize_search_query(test_query)
    print(f"Original: {test_query}")
    print(f"Optimized: {optimized}")
    
    # Test query validation
    validation_result, validation_msg = optimizer.validate_query_safety(optimized)
    print(f"Validation: {validation_result} - {validation_msg}")
    
    # Test chunking
    large_request = "This is a very large request that needs to be chunked into smaller pieces to fit within the context window limits. It contains multiple sentences and ideas that should be processed separately."
    chunks = optimizer.chunk_large_request(large_request)
    print(f"Original request: {large_request}")
    print(f"Chunked into {len(chunks)} pieces:")
    for i, chunk in enumerate(chunks):
        print(f"  Chunk {i+1}: {chunk}")

if __name__ == "__main__":
    main()
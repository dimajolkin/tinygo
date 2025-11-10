#!/usr/bin/env python3
"""
Context Manager for TinyGo Project
Prevents ContextWindowExceededError by managing token usage and fallback strategies.
"""

import json
import os
import sys
from typing import Dict, List, Optional, Any
import logging

class ContextManager:
    """Manages context window usage and prevents overflow."""
    
    def __init__(self, config_path: str = ".kilocode/config.json"):
        self.config = self._load_config(config_path)
        self.logger = self._setup_logger()
        self.current_tokens = 0
        self.max_tokens = self.config.get("model_settings", {}).get("context_window_limit", 128000)
        self.max_tokens_per_request = self.config.get("model_settings", {}).get("max_tokens_per_request", 4000)
        self.chunk_size = self.config.get("model_settings", {}).get("chunk_size", 500)
        
    def _load_config(self, config_path: str) -> Dict[str, Any]:
        """Load configuration from JSON file."""
        try:
            with open(config_path, 'r', encoding='utf-8') as f:
                return json.load(f)
        except FileNotFoundError:
            return self._get_default_config()
        except json.JSONDecodeError as e:
            print(f"Error loading config: {e}")
            return self._get_default_config()
    
    def _get_default_config(self) -> Dict[str, Any]:
        """Get default configuration."""
        return {
            "model_settings": {
                "context_window_limit": 128000,
                "max_tokens_per_request": 4000,
                "chunk_size": 500
            },
            "analysis_settings": {
                "max_file_size_read": 1000,
                "use_semantic_search_first": True,
                "max_recursive_depth": 3
            },
            "error_handling": {
                "context_window_exceeded": {
                    "retry_with_smaller_context": True,
                    "fallback_to_alternative_model": True,
                    "chunk_and_retry": True
                }
            }
        }
    
    def _setup_logger(self) -> logging.Logger:
        """Setup logging."""
        logger = logging.getLogger("ContextManager")
        logger.setLevel(logging.INFO)
        
        handler = logging.StreamHandler()
        formatter = logging.Formatter(
            '%(asctime)s - %(name)s - %(levelname)s - %(message)s'
        )
        handler.setFormatter(formatter)
        logger.addHandler(handler)
        
        return logger
    
    def can_add_request(self, estimated_tokens: int) -> bool:
        """Check if adding a request would exceed context limits."""
        estimated_total = self.current_tokens + estimated_tokens
        safety_margin = self.max_tokens * 0.95  # Keep 5% safety margin
        
        return estimated_total <= safety_margin
    
    def add_request_tokens(self, tokens: int):
        """Add tokens to current usage."""
        self.current_tokens += tokens
        self.logger.debug(f"Current token usage: {self.current_tokens}/{self.max_tokens}")
    
    def get_chunk_size(self) -> int:
        """Get optimal chunk size for file reading."""
        return self.chunk_size
    
    def should_chunk_file(self, file_path: str) -> bool:
        """Determine if a file should be chunked."""
        try:
            file_size = os.path.getsize(file_path)
            max_size = self.config.get("analysis_settings", {}).get("max_file_size_read", 1000)
            return file_size > max_size * 1024  # Convert KB to bytes
        except OSError:
            return True  # Default to chunking if we can't check size
    
    def handle_context_exceeded(self, error_message: str) -> Dict[str, Any]:
        """Handle context window exceeded error."""
        self.logger.warning(f"Context window exceeded: {error_message}")
        
        error_handling = self.config.get("error_handling", {}).get("context_window_exceeded", {})
        
        strategies = []
        
        if error_handling.get("retry_with_smaller_context", True):
            strategies.append({
                "strategy": "reduce_context",
                "action": "reduce_tokens_per_request",
                "new_limit": int(self.max_tokens_per_request * 0.8)
            })
        
        if error_handling.get("fallback_to_alternative_model", True):
            fallback_models = self.config.get("model_settings", {}).get("fallback_models", [])
            if fallback_models:
                strategies.append({
                    "strategy": "model_fallback",
                    "action": "switch_to_alternative_model",
                    "available_models": fallback_models
                })
        
        if error_handling.get("chunk_and_retry", True):
            strategies.append({
                "strategy": "chunking",
                "action": "divide_request_into_smaller_parts",
                "chunk_size": self.chunk_size
            })
        
        return {
            "error": "ContextWindowExceededError",
            "strategies": strategies,
            "current_usage": self.current_tokens,
            "max_limit": self.max_tokens
        }
    
    def reset_context(self):
        """Reset token counter."""
        self.current_tokens = 0
        self.logger.info("Context token counter reset")
    
    def get_usage_info(self) -> Dict[str, int]:
        """Get current context usage information."""
        return {
            "current_tokens": self.current_tokens,
            "max_tokens": self.max_tokens,
            "percentage_used": (self.current_tokens / self.max_tokens) * 100
        }
    
    def should_use_semantic_search(self) -> bool:
        """Check if semantic search should be used first."""
        return self.config.get("analysis_settings", {}).get("use_semantic_search_first", True)
    
    def get_max_recursive_depth(self) -> int:
        """Get maximum recursion depth for analysis."""
        return self.config.get("analysis_settings", {}).get("max_recursive_depth", 3)

def main():
    """Main function for testing."""
    if len(sys.argv) > 1:
        config_path = sys.argv[1]
    else:
        config_path = ".kilocode/config.json"
    
    manager = ContextManager(config_path)
    
    print("Context Manager initialized")
    print(f"Max tokens: {manager.max_tokens}")
    print(f"Current usage: {manager.current_tokens}")
    print(f"Chunk size: {manager.chunk_size}")
    
    # Test context management
    test_tokens = 5000
    if manager.can_add_request(test_tokens):
        print(f"Can add request with {test_tokens} tokens")
        manager.add_request_tokens(test_tokens)
    else:
        print(f"Cannot add request with {test_tokens} tokens")
    
    print(f"Usage info: {manager.get_usage_info()}")

if __name__ == "__main__":
    main()
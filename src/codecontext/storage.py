from __future__ import annotations

from typing import Any, Protocol

from .db import Dependency, Entity, EntityRelation, File


class StorageBackend(Protocol):
    @classmethod
    def open(cls, db_path: str, verbose: bool = False) -> "StorageBackend":
        ...

    def close(self) -> None:
        ...

    def commit(self) -> None:
        ...

    def insert_file(self, path: str, language: str, file_hash: str, lines_of_code: int, tokens: int) -> int:
        ...

    def get_file_by_path(self, path: str) -> File | None:
        ...

    def get_file_by_id(self, file_id: int) -> File | None:
        ...

    def get_files(self) -> list[File]:
        ...

    def insert_entity(
        self,
        file_id: int,
        name: str,
        entity_type: str,
        kind: str,
        signature: str,
        start_line: int,
        end_line: int,
        docs: str,
        parent: str,
        visibility: str,
        scope: str,
        language: str,
    ) -> int:
        ...

    def get_entities_by_file(self, file_id: int) -> list[Entity]:
        ...

    def get_entity_by_name(self, name: str) -> list[Entity]:
        ...

    def get_entity_by_id(self, entity_id: int) -> Entity | None:
        ...

    def get_all_entities(self) -> list[Entity]:
        ...

    def delete_entities_by_file(self, file_id: int) -> None:
        ...

    def delete_dependencies_by_file(self, file_id: int) -> None:
        ...

    def batch_insert_entities(self, file_id: int, rows: list[dict[str, Any]]) -> list[int]:
        ...

    def batch_insert_dependencies(self, file_id: int, rows: list[dict[str, Any]]) -> int:
        ...

    def batch_insert_relations(self, rows: list[tuple[int, int, str, int, str]]) -> int:
        ...

    def insert_dependency(self, source_file_id: int, target_path: str, dep_type: str, line_number: int) -> int:
        ...

    def get_dependencies(self, file_id: int) -> list[Dependency]:
        ...

    def get_all_dependencies(self) -> list[Dependency]:
        ...

    def insert_entity_relation(
        self,
        source_entity_id: int,
        target_entity_id: int,
        relation_type: str,
        line_number: int,
        context: str,
    ) -> int:
        ...

    def get_entity_relations(self, entity_id: int, relation_type: str = "") -> list[EntityRelation]:
        ...

    def get_inbound_entity_relations(self, entity_id: int, relation_type: str = "") -> list[EntityRelation]:
        ...

    def get_all_relations(self) -> list[EntityRelation]:
        ...

    def search_entities(self, query: str, limit: int = 20) -> list[Entity]:
        ...

    def get_file_imports(self, path: str) -> list[str]:
        ...

    def get_call_graph(self, entity_id: int, depth: int = 1) -> dict[str, Any]:
        ...

    def get_dependency_graph(self, file_path: str) -> dict[str, Any]:
        ...

    def get_file_count(self) -> int:
        ...

    def get_lines_of_code_count(self) -> int:
        ...

    def get_tokens_count(self) -> int:
        ...

    def get_entity_count(self) -> int:
        ...

    def get_dependency_count(self) -> int:
        ...

    def get_relation_count(self) -> int:
        ...
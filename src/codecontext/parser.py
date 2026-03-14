from __future__ import annotations

import ast
import re
from dataclasses import dataclass, field
from pathlib import Path


@dataclass
class Entity:
    name: str
    type: str
    kind: str = ""
    signature: str = ""
    start_line: int = 0
    end_line: int = 0
    column_start: int = 0
    column_end: int = 0
    docs: str = ""
    parent: str = ""
    visibility: str = ""
    scope: str = ""
    language: str = ""
    attributes: dict[str, object] = field(default_factory=dict)


@dataclass
class Dependency:
    path: str
    type: str
    line_number: int
    resolved: str = ""
    is_local: bool = False


@dataclass
class ParseResult:
    file_path: str
    language: str
    entities: list[Entity]
    dependencies: list[Dependency]


def detect_language(file_path: str) -> str:
    ext = Path(file_path).suffix.lower()
    if ext == ".go":
        return "go"
    if ext == ".py":
        return "python"
    if ext in {".js", ".jsx"}:
        return "javascript"
    if ext in {".ts", ".tsx"}:
        return "typescript"
    if ext == ".java":
        return "java"
    return "unknown"


def parse(file_path: str, content: str) -> ParseResult:
    language = detect_language(file_path)
    if language == "python":
        return _parse_python(file_path, content)
    if language == "go":
        return _parse_go(file_path, content)
    if language in {"javascript", "typescript"}:
        return _parse_js(file_path, content, language)
    if language == "java":
        return _parse_java(file_path, content)
    return ParseResult(file_path=file_path, language=language, entities=[], dependencies=[])


def _parse_python(file_path: str, content: str) -> ParseResult:
    entities: list[Entity] = []
    dependencies: list[Dependency] = []

    try:
        tree = ast.parse(content)
    except SyntaxError:
        return ParseResult(file_path=file_path, language="python", entities=[], dependencies=[])

    class Visitor(ast.NodeVisitor):
        def __init__(self) -> None:
            self.parent_stack: list[str] = []

        def visit_ClassDef(self, node: ast.ClassDef) -> None:  # noqa: N802
            entities.append(
                Entity(
                    name=node.name,
                    type="class",
                    kind="class",
                    signature=f"class {node.name}",
                    start_line=node.lineno,
                    end_line=getattr(node, "end_lineno", node.lineno),
                    docs=(ast.get_docstring(node) or ""),
                    parent=self.parent_stack[-1] if self.parent_stack else "",
                    visibility="private" if node.name.startswith("_") else "public",
                    scope="module",
                    language="python",
                )
            )
            self.parent_stack.append(node.name)
            self.generic_visit(node)
            self.parent_stack.pop()

        def visit_FunctionDef(self, node: ast.FunctionDef) -> None:  # noqa: N802
            parent = self.parent_stack[-1] if self.parent_stack else ""
            entities.append(
                Entity(
                    name=node.name,
                    type="method" if parent else "function",
                    kind="function",
                    signature=f"def {node.name}(...)",
                    start_line=node.lineno,
                    end_line=getattr(node, "end_lineno", node.lineno),
                    docs=(ast.get_docstring(node) or ""),
                    parent=parent,
                    visibility="private" if node.name.startswith("_") else "public",
                    scope="class" if parent else "module",
                    language="python",
                )
            )
            self.generic_visit(node)

        def visit_AsyncFunctionDef(self, node: ast.AsyncFunctionDef) -> None:  # noqa: N802
            parent = self.parent_stack[-1] if self.parent_stack else ""
            entities.append(
                Entity(
                    name=node.name,
                    type="method" if parent else "function",
                    kind="async_function",
                    signature=f"async def {node.name}(...)",
                    start_line=node.lineno,
                    end_line=getattr(node, "end_lineno", node.lineno),
                    docs=(ast.get_docstring(node) or ""),
                    parent=parent,
                    visibility="private" if node.name.startswith("_") else "public",
                    scope="class" if parent else "module",
                    language="python",
                )
            )
            self.generic_visit(node)

        def visit_Import(self, node: ast.Import) -> None:  # noqa: N802
            for alias in node.names:
                path = alias.name
                dependencies.append(
                    Dependency(
                        path=path,
                        type="import",
                        line_number=node.lineno,
                        is_local=path.startswith("."),
                    )
                )

        def visit_ImportFrom(self, node: ast.ImportFrom) -> None:  # noqa: N802
            module = node.module or ""
            dot_prefix = "." * node.level
            path = f"{dot_prefix}{module}" if (dot_prefix or module) else ""
            dependencies.append(
                Dependency(
                    path=path,
                    type="from",
                    line_number=node.lineno,
                    is_local=node.level > 0,
                )
            )

    Visitor().visit(tree)
    return ParseResult(file_path=file_path, language="python", entities=entities, dependencies=dependencies)


def _parse_go(file_path: str, content: str) -> ParseResult:
    entities: list[Entity] = []
    dependencies: list[Dependency] = []

    func_re = re.compile(r"^\s*func\s+(?:\([^)]*\)\s*)?([A-Za-z_][A-Za-z0-9_]*)")
    type_re = re.compile(r"^\s*type\s+([A-Za-z_][A-Za-z0-9_]*)\s+(struct|interface)")
    import_single_re = re.compile(r'^\s*import\s+"([^"]+)"')
    import_multi_re = re.compile(r'^\s*"([^"]+)"\s*$')

    in_import_block = False
    for i, line in enumerate(content.splitlines(), start=1):
        if line.strip().startswith("import ("):
            in_import_block = True
            continue
        if in_import_block and line.strip() == ")":
            in_import_block = False
            continue

        m = func_re.match(line)
        if m:
            name = m.group(1)
            entities.append(
                Entity(
                    name=name,
                    type="function",
                    kind="function",
                    signature=f"func {name}",
                    start_line=i,
                    end_line=i,
                    visibility="private" if name[:1].islower() else "public",
                    scope="package",
                    language="go",
                )
            )

        m = type_re.match(line)
        if m:
            name = m.group(1)
            kind = m.group(2)
            entities.append(
                Entity(
                    name=name,
                    type=kind,
                    kind=kind,
                    signature=f"type {name} {kind}",
                    start_line=i,
                    end_line=i,
                    visibility="private" if name[:1].islower() else "public",
                    scope="package",
                    language="go",
                )
            )

        m = import_single_re.match(line)
        if m:
            dependencies.append(Dependency(path=m.group(1), type="import", line_number=i, is_local=False))

        if in_import_block:
            m = import_multi_re.match(line)
            if m:
                dependencies.append(Dependency(path=m.group(1), type="import", line_number=i, is_local=False))

    return ParseResult(file_path=file_path, language="go", entities=entities, dependencies=dependencies)


def _parse_js(file_path: str, content: str, language: str) -> ParseResult:
    entities: list[Entity] = []
    dependencies: list[Dependency] = []

    fn_re = re.compile(r"^\s*function\s+([A-Za-z_][A-Za-z0-9_]*)")
    class_re = re.compile(r"^\s*class\s+([A-Za-z_][A-Za-z0-9_]*)")
    import_re = re.compile(r"""^\s*import\s+[^'"]*from\s+['"]([^'"]+)['"]""")
    require_re = re.compile(r"require\(['\"]([^'\"]+)['\"]\)")

    for i, line in enumerate(content.splitlines(), start=1):
        fn = fn_re.match(line)
        if fn:
            name = fn.group(1)
            entities.append(
                Entity(
                    name=name,
                    type="function",
                    kind="function",
                    signature=f"function {name}",
                    start_line=i,
                    end_line=i,
                    visibility="private",
                    scope="module",
                    language=language,
                )
            )

        cls = class_re.match(line)
        if cls:
            name = cls.group(1)
            entities.append(
                Entity(
                    name=name,
                    type="class",
                    kind="class",
                    signature=f"class {name}",
                    start_line=i,
                    end_line=i,
                    visibility="public",
                    scope="module",
                    language=language,
                )
            )

        imp = import_re.match(line)
        if imp:
            dep = imp.group(1)
            dependencies.append(
                Dependency(path=dep, type="import", line_number=i, is_local=dep.startswith(".") or dep.startswith("/"))
            )

        req = require_re.search(line)
        if req:
            dep = req.group(1)
            dependencies.append(
                Dependency(path=dep, type="require", line_number=i, is_local=dep.startswith(".") or dep.startswith("/"))
            )

    return ParseResult(file_path=file_path, language=language, entities=entities, dependencies=dependencies)


def _parse_java(file_path: str, content: str) -> ParseResult:
    entities: list[Entity] = []
    dependencies: list[Dependency] = []

    class_re = re.compile(r"^\s*(public\s+)?(class|interface|enum)\s+([A-Za-z_][A-Za-z0-9_]*)")
    method_re = re.compile(
        r"^\s*(?:(?:public|private|protected|static|final|abstract|synchronized|native)\s+)*"
        r"[A-Za-z0-9_<>\[\]]+\s+"
        r"([A-Za-z_][A-Za-z0-9_]*)\s*\("
    )
    import_re = re.compile(r"^\s*import\s+([^;]+);")

    current_class = ""
    for i, line in enumerate(content.splitlines(), start=1):
        imp = import_re.match(line)
        if imp:
            dep = imp.group(1).strip()
            dependencies.append(Dependency(path=dep, type="import", line_number=i, is_local=False))

        cls = class_re.match(line)
        if cls:
            kind = cls.group(2)
            name = cls.group(3)
            current_class = name
            entities.append(
                Entity(
                    name=name,
                    type=kind,
                    kind=kind,
                    signature=f"{kind} {name}",
                    start_line=i,
                    end_line=i,
                    visibility="public" if cls.group(1) else "package",
                    scope="package",
                    language="java",
                )
            )
            continue

        method = method_re.match(line)
        if method and " class " not in f" {line} ":
            name = method.group(1)
            # Detect visibility from the line text directly.
            vis = "public" if "public " in line else "private" if "private " in line else "protected" if "protected " in line else "package"
            entities.append(
                Entity(
                    name=name,
                    type="method",
                    kind="method",
                    signature=f"{name}(...)",
                    start_line=i,
                    end_line=i,
                    parent=current_class,
                    visibility=vis,
                    scope="class" if current_class else "package",
                    language="java",
                )
            )

    return ParseResult(file_path=file_path, language="java", entities=entities, dependencies=dependencies)

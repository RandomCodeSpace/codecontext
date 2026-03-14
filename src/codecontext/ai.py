from __future__ import annotations

from dataclasses import dataclass, field

from .indexer import Indexer
from .llm import Message, Provider


@dataclass
class ConversationMessage:
	role: str
	content: str


@dataclass
class ConversationContext:
	messages: list[ConversationMessage] = field(default_factory=list)
	entity_id: int = 0
	entity_name: str = ""
	file_path: str = ""


class Chain:
	def __init__(self, indexer: Indexer, provider: Provider):
		self.indexer = indexer
		self.provider = provider

	def query_natural(self, query: str) -> str:
		stats = self.indexer.get_stats()
		system_prompt = (
			"You are a code analysis AI assistant. You have access to a code graph database.\n"
			f"Current graph contains: {stats['files']} files, {stats['entities']} entities, {stats['dependencies']} dependencies.\n"
			"When answering questions about code, provide specific references to entities in the codebase."
		)
		return self.provider.chat([
			Message(role="system", content=system_prompt),
			Message(role="user", content=query),
		])

	def analyze_entity(self, entity_name: str) -> str:
		entities = self.indexer.query_entity(entity_name)
		if not entities:
			raise ValueError(f"entity not found: {entity_name}")
		entity = entities[0]
		call_graph = self.indexer.query_call_graph(entity.id)
		file = self.indexer.get_file_by_id(entity.file_id)
		file_path = file.path if file else ""
		context = (
			"Analyze this code entity:\n"
			f"Name: {entity.name}\n"
			f"Type: {entity.type}\n"
			f"Kind: {entity.kind}\n"
			f"Signature: {entity.signature}\n"
			f"Location: {file_path} (lines {entity.start_line}-{entity.end_line})\n"
			f"Documentation: {entity.documentation}\n"
			f"Calls: {call_graph.get('calls', [])}\n"
		)
		return self.provider.chat([
			Message(role="system", content="You are a code analysis expert. Provide detailed analysis including purpose, dependencies, and potential issues."),
			Message(role="user", content=context),
		])

	def generate_docs(self, entity_name: str) -> str:
		entities = self.indexer.query_entity(entity_name)
		if not entities:
			raise ValueError(f"entity not found: {entity_name}")
		entity = entities[0]
		file = self.indexer.get_file_by_id(entity.file_id)
		file_path = file.path if file else ""
		context = (
			"Generate comprehensive documentation for this code entity:\n\n"
			f"Name: {entity.name}\n"
			f"Type: {entity.type}\n"
			f"Kind: {entity.kind}\n"
			f"Signature: {entity.signature}\n"
			f"Location: {file_path} (lines {entity.start_line}-{entity.end_line})\n"
			f"Current Documentation: {entity.documentation}\n"
		)
		return self.provider.chat([
			Message(
				role="system",
				content=(
					"You are a technical documentation expert. Include purpose, parameters/return values, examples, errors, and related entities."
				),
			),
			Message(role="user", content=context),
		])

	def review_code(self, entity_name: str) -> str:
		entities = self.indexer.query_entity(entity_name)
		if not entities:
			raise ValueError(f"entity not found: {entity_name}")
		entity = entities[0]
		file = self.indexer.get_file_by_id(entity.file_id)
		file_path = file.path if file else ""
		context = (
			"Review this code for quality and best practices:\n\n"
			f"Name: {entity.name}\n"
			f"Type: {entity.type}\n"
			f"Kind: {entity.kind}\n"
			f"Signature: {entity.signature}\n"
			f"Location: {file_path} (lines {entity.start_line}-{entity.end_line})\n"
		)
		return self.provider.chat([
			Message(
				role="system",
				content=(
					"You are an experienced code reviewer. Provide feedback on quality, performance, best practices, bugs, and improvements."
				),
			),
			Message(role="user", content=context),
		])

	def summarize(self, file_path: str) -> str:
		deps = self.indexer.query_dependency_graph(file_path)
		context = (
			"Summarize the purpose and structure of this file:\n"
			f"Path: {file_path}\n"
			f"Dependencies: {deps.get('dependencies', [])}\n"
		)
		return self.provider.chat([
			Message(
				role="system",
				content=(
					"You are a code documentation expert. Summarize purpose, key components, dependencies, and use cases."
				),
			),
			Message(role="user", content=context),
		])

	def generate_project_docs(self, prompt_instruction: str = "") -> str:
		files = self.indexer.get_all_files()
		entities = self.indexer.get_all_entities()
		by_file: dict[int, list[object]] = {}
		for entity in entities:
			by_file.setdefault(entity.file_id, []).append(entity)

		if not prompt_instruction:
			prompt_instruction = (
				"You are a technical documentation writer. For each code entity listed, write clear and concise documentation in Markdown."
			)

		parts: list[str] = ["# Project Documentation (AI-generated)", ""]
		for file in files:
			ents = by_file.get(file.id, [])
			if not ents:
				continue

			lines = [f"File: {file.path}  Language: {file.language}", "", "Entities:"]
			for entity in ents:
				row = f"  [{entity.type}] {entity.name}"
				if entity.signature:
					row += f"  signature: {entity.signature}"
				if entity.parent:
					row += f"  parent: {entity.parent}"
				row += f"  lines {entity.start_line}-{entity.end_line}"
				lines.append(row)

			try:
				response = self.provider.chat([
					Message(role="system", content=prompt_instruction),
					Message(role="user", content="\n".join(lines)),
				])
			except Exception as err:  # noqa: BLE001
				response = f"_Error generating docs: {err}_"

			parts.append(f"## `{file.path}`")
			parts.append("")
			parts.append(response)
			parts.append("")
			parts.append("---")
			parts.append("")

		return "\n".join(parts)

	def chat(self, conversation: ConversationContext, user_message: str) -> str:
		conversation.messages.append(ConversationMessage(role="user", content=user_message))
		llm_messages = [Message(role=m.role, content=m.content) for m in conversation.messages]
		response = self.provider.chat(llm_messages)
		conversation.messages.append(ConversationMessage(role="assistant", content=response))
		return response

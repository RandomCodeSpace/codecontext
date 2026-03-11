package parser_test

import (
	"testing"

	"github.com/RandomCodeSpace/codecontext/pkg/parser"
)

// ---------------------------------------------------------------------------
// Python
// ---------------------------------------------------------------------------

const pySource = `
import os
import sys as system
from typing import List, Optional

class Animal:
    """Base animal class."""

    def __init__(self, name: str):
        """Initialise."""
        self.name = name

    async def speak(self) -> str:
        """Make noise."""
        return ""

class Dog(Animal):
    def fetch(self, item: str) -> bool:
        return True

def standalone(x: int, y: int) -> int:
    return x + y
`

func TestPythonParser(t *testing.T) {
	result, err := parser.Parse("example.py", pySource)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Dependencies
	wantDeps := map[string]string{
		"os":     "import",
		"sys":    "import",
		"typing": "from",
	}
	for _, d := range result.Dependencies {
		wantType, ok := wantDeps[d.Path]
		if !ok {
			t.Errorf("unexpected dependency: %q", d.Path)
			continue
		}
		if d.Type != wantType {
			t.Errorf("dep %q: want type %q, got %q", d.Path, wantType, d.Type)
		}
		delete(wantDeps, d.Path)
	}
	for path := range wantDeps {
		t.Errorf("missing dependency: %q", path)
	}

	// Entities
	byName := make(map[string]*parser.Entity)
	for _, e := range result.Entities {
		byName[e.Name] = e
	}

	// Classes
	for _, name := range []string{"Animal", "Dog"} {
		e, ok := byName[name]
		if !ok {
			t.Errorf("missing class %q", name)
			continue
		}
		if e.Type != "class" {
			t.Errorf("%s: want type=class, got %q", name, e.Type)
		}
	}

	// Methods with correct parent
	for name, wantParent := range map[string]string{
		"__init__": "Animal",
		"speak":    "Animal",
		"fetch":    "Dog",
	} {
		e, ok := byName[name]
		if !ok {
			t.Errorf("missing method %q", name)
			continue
		}
		if e.Type != "method" {
			t.Errorf("%s: want type=method, got %q", name, e.Type)
		}
		if e.Parent != wantParent {
			t.Errorf("%s: want parent=%q, got %q", name, wantParent, e.Parent)
		}
	}

	// Async method
	if e, ok := byName["speak"]; ok {
		if e.Kind != "async_method" {
			t.Errorf("speak: want kind=async_method, got %q", e.Kind)
		}
	}

	// Top-level function
	if e, ok := byName["standalone"]; ok {
		if e.Type != "function" {
			t.Errorf("standalone: want type=function, got %q", e.Type)
		}
		if e.Parent != "" {
			t.Errorf("standalone: want no parent, got %q", e.Parent)
		}
	} else {
		t.Error("missing function 'standalone'")
	}

	// End lines: Animal.__init__ must end before Animal class end.
	if animal, ok := byName["Animal"]; ok {
		if init, ok := byName["__init__"]; ok {
			if init.EndLine >= animal.EndLine {
				t.Errorf("__init__.EndLine=%d should be < Animal.EndLine=%d",
					init.EndLine, animal.EndLine)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// JavaScript
// ---------------------------------------------------------------------------

const jsSource = `
import React from 'react';
import { useState } from 'react';
const path = require('path');

class EventEmitter {
    constructor(name) {
        this.name = name;
    }

    async emit(event, data) {
        // emit event
    }

    static create(name) {
        return new EventEmitter(name);
    }
}

function greet(name) {
    return 'Hello ' + name;
}

const add = (a, b) => a + b;

export async function fetchData(url) {
    return fetch(url);
}
`

func TestJavaScriptParser(t *testing.T) {
	result, err := parser.Parse("example.js", jsSource)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Dependencies
	wantDeps := map[string]bool{"react": true, "path": true}
	for _, d := range result.Dependencies {
		delete(wantDeps, d.Path)
	}
	for path := range wantDeps {
		t.Errorf("missing dependency: %q", path)
	}

	byName := make(map[string]*parser.Entity)
	for _, e := range result.Entities {
		byName[e.Name] = e
	}

	// Class
	if e, ok := byName["EventEmitter"]; !ok {
		t.Error("missing class EventEmitter")
	} else if e.Type != "class" {
		t.Errorf("EventEmitter: want type=class, got %q", e.Type)
	}

	// Methods inside the class
	for name, wantParent := range map[string]string{
		"constructor": "EventEmitter",
		"emit":        "EventEmitter",
		"create":      "EventEmitter",
	} {
		e, ok := byName[name]
		if !ok {
			t.Errorf("missing method %q", name)
			continue
		}
		if e.Parent != wantParent {
			t.Errorf("%s: want parent=%q, got %q", name, wantParent, e.Parent)
		}
	}

	// Top-level function
	if e, ok := byName["greet"]; !ok {
		t.Error("missing function 'greet'")
	} else if e.Type != "function" {
		t.Errorf("greet: want type=function, got %q", e.Type)
	}

	// Arrow function
	if e, ok := byName["add"]; !ok {
		t.Error("missing arrow function 'add'")
	} else if e.Kind != "arrow_function" {
		t.Errorf("add: want kind=arrow_function, got %q", e.Kind)
	}

	// Async exported function
	if e, ok := byName["fetchData"]; !ok {
		t.Error("missing function 'fetchData'")
	} else if e.Kind != "async_function" {
		t.Errorf("fetchData: want kind=async_function, got %q", e.Kind)
	}

	// End lines: EventEmitter methods must be inside class span.
	if cls, ok := byName["EventEmitter"]; ok {
		for _, name := range []string{"constructor", "emit", "create"} {
			if m, ok := byName[name]; ok {
				if m.StartLine < cls.StartLine || m.EndLine > cls.EndLine {
					t.Errorf("%s [%d-%d] not inside EventEmitter [%d-%d]",
						name, m.StartLine, m.EndLine, cls.StartLine, cls.EndLine)
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Java
// ---------------------------------------------------------------------------

const javaSource = `
package com.example;

import java.util.List;
import java.util.ArrayList;

public class Repository<T> {

    private final List<T> items = new ArrayList<>();

    public void add(T item) {
        items.add(item);
    }

    public List<T> findAll() {
        return items;
    }

    public interface Finder<T> {
        List<T> findBy(String criteria);
    }
}

enum Status {
    ACTIVE, INACTIVE
}
`

func TestJavaParser(t *testing.T) {
	result, err := parser.Parse("example.java", javaSource)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Dependencies
	wantDeps := map[string]bool{
		"java.util.List":      true,
		"java.util.ArrayList": true,
	}
	for _, d := range result.Dependencies {
		delete(wantDeps, d.Path)
	}
	for path := range wantDeps {
		t.Errorf("missing dependency: %q", path)
	}

	byName := make(map[string]*parser.Entity)
	for _, e := range result.Entities {
		byName[e.Name] = e
	}

	// Class
	if e, ok := byName["Repository"]; !ok {
		t.Error("missing class Repository")
	} else if e.Type != "class" {
		t.Errorf("Repository: want type=class, got %q", e.Type)
	}

	// Interface nested inside class
	if e, ok := byName["Finder"]; !ok {
		t.Error("missing interface Finder")
	} else {
		if e.Type != "interface" {
			t.Errorf("Finder: want type=interface, got %q", e.Type)
		}
		if e.Parent != "Repository" {
			t.Errorf("Finder: want parent=Repository, got %q", e.Parent)
		}
	}

	// Enum at top level
	if e, ok := byName["Status"]; !ok {
		t.Error("missing enum Status")
	} else if e.Type != "enum" {
		t.Errorf("Status: want type=enum, got %q", e.Type)
	}

	// Methods
	for name, wantParent := range map[string]string{
		"add":     "Repository",
		"findAll": "Repository",
	} {
		e, ok := byName[name]
		if !ok {
			t.Errorf("missing method %q", name)
			continue
		}
		if e.Type != "method" {
			t.Errorf("%s: want type=method, got %q", name, e.Type)
		}
		if e.Parent != wantParent {
			t.Errorf("%s: want parent=%q, got %q", name, wantParent, e.Parent)
		}
	}

	// End lines: methods must be inside Repository span.
	if cls, ok := byName["Repository"]; ok {
		for _, name := range []string{"add", "findAll"} {
			if m, ok := byName[name]; ok {
				if m.StartLine < cls.StartLine || m.EndLine > cls.EndLine {
					t.Errorf("%s [%d-%d] not inside Repository [%d-%d]",
						name, m.StartLine, m.EndLine, cls.StartLine, cls.EndLine)
				}
			}
		}
	}
}

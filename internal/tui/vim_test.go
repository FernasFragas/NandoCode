package tui

import "testing"

func TestVimCommandStateTransitions(t *testing.T) {
	v := NewVimState()
	v.EnterNormal()

	v.HandleNormalKey('3')
	if _, ok := v.CommandState.(CmdCount); !ok {
		t.Fatalf("expected CmdCount, got %T", v.CommandState)
	}
	v.HandleNormalKey('d')
	if st, ok := v.CommandState.(CmdOperator); !ok || st.Op != OpDelete || st.Count != 3 {
		t.Fatalf("expected CmdOperator delete count=3, got %#v", v.CommandState)
	}
	v.HandleNormalKey('w')
	if _, ok := v.CommandState.(CmdIdle); !ok {
		t.Fatalf("expected CmdIdle after motion completion, got %T", v.CommandState)
	}

	v.HandleNormalKey('d')
	v.HandleNormalKey('2')
	if st, ok := v.CommandState.(CmdOperatorCount); !ok || st.Digits != "2" {
		t.Fatalf("expected CmdOperatorCount, got %#v", v.CommandState)
	}
	v.HandleNormalKey('f')
	if _, ok := v.CommandState.(CmdOperatorFind); !ok {
		t.Fatalf("expected CmdOperatorFind, got %T", v.CommandState)
	}
	v.HandleNormalKey('x')
	if _, ok := v.CommandState.(CmdIdle); !ok {
		t.Fatalf("expected CmdIdle after operator-find char, got %T", v.CommandState)
	}

	v.HandleNormalKey('c')
	v.HandleNormalKey('i')
	if st, ok := v.CommandState.(CmdOperatorTextObj); !ok || st.Scope != TextObjInner {
		t.Fatalf("expected CmdOperatorTextObj inner, got %#v", v.CommandState)
	}
	v.HandleNormalKey('"')
	if _, ok := v.CommandState.(CmdIdle); !ok {
		t.Fatalf("expected CmdIdle after text object delimiter, got %T", v.CommandState)
	}

	v.HandleNormalKey('f')
	if _, ok := v.CommandState.(CmdFind); !ok {
		t.Fatalf("expected CmdFind, got %T", v.CommandState)
	}
	v.HandleNormalKey('a')
	if _, ok := v.CommandState.(CmdIdle); !ok {
		t.Fatalf("expected CmdIdle after find target, got %T", v.CommandState)
	}

	v.HandleNormalKey('g')
	if _, ok := v.CommandState.(CmdGPrefix); !ok {
		t.Fatalf("expected CmdGPrefix, got %T", v.CommandState)
	}
	v.HandleNormalKey('g')
	if _, ok := v.CommandState.(CmdIdle); !ok {
		t.Fatalf("expected CmdIdle after gg, got %T", v.CommandState)
	}

	v.HandleNormalKey('r')
	if _, ok := v.CommandState.(CmdReplace); !ok {
		t.Fatalf("expected CmdReplace, got %T", v.CommandState)
	}
	v.HandleNormalKey('z')
	if _, ok := v.CommandState.(CmdIdle); !ok {
		t.Fatalf("expected CmdIdle after replace char, got %T", v.CommandState)
	}

	v.HandleNormalKey('>')
	if st, ok := v.CommandState.(CmdIndent); !ok || st.Dir != OpIndent {
		t.Fatalf("expected CmdIndent >, got %#v", v.CommandState)
	}
	v.HandleNormalKey('>')
	if _, ok := v.CommandState.(CmdIdle); !ok {
		t.Fatalf("expected CmdIdle after >>, got %T", v.CommandState)
	}
}


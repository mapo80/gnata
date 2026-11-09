package parser

// NodeType is the kind of a node (see the Node* constants). It prints as
// the JSONata node type name ("path", "binary", ...).
type NodeType uint8

// String returns the JSONata name of the node type ("" for the zero value).
func (t NodeType) String() string {
	if int(t) < len(nodeTypeNames) {
		return nodeTypeNames[t]
	}
	return ""
}

// Node represents one AST node produced by the parser.
// Fields are populated based on Type; unused fields are zero-valued.
//
// The fields most nodes use are inline; the ones only a few node types use
// live in one structure behind ext, read through the methods of the same
// name, so that a node fits the 144-byte size class.
type Node struct {
	Type NodeType // node type constant (see Node* constants)

	// Metadata flags set by processAST
	KeepArray          bool // step has [] suffix → force array output
	KeepSingletonArray bool // path has at least one keepArray step
	ConsArray          bool // step is an array constructor used as a path step
	Thunk              bool // lambda is a TCO thunk

	Value string // operator string, field name, variable name, or literal value
	Pos   int    // byte offset in source

	// Numeric literal value (populated when Type==NodeNumber)
	NumVal float64

	// Binary operands
	Left  *Node
	Right *Node

	Procedure *Node // type="function"|"partial"

	// Path / steps
	Steps []*Node // type="path": ordered list of path steps

	// Compound nodes
	Expressions []*Node // type="block" or type="unary"([)
	Arguments   []*Node // type="function"|"partial"|"lambda"

	// ext holds the fields only some node types use; nil when all are zero.
	ext *nodeExt
}

// nodeExt holds the fields of a Node that only some node types use.
type nodeExt struct {
	// Unary operand (-)
	Expression *Node
	// type="lambda"
	Body *Node

	// Condition / then / else
	Condition *Node
	Then      *Node
	Else      *Node // nil if no else branch

	// Transform: type="transform"
	Pattern *Node
	Update  *Node
	Delete  *Node // nil if no delete clause

	LHS []*Node // type="unary"({): key-value pairs flat [k0,v0,k1,v1,...]

	// Sort: type="sort"
	Terms []SortTerm

	// Lambda signature
	Signature *Signature // parsed signature; nil if absent

	// Focus / index variable names (set by @ and # operators)
	Focus string // variable name bound by @
	Index string // variable name bound by #

	// Group expression attached to a path or step
	Group *GroupExpr
}

// empty reports whether every field of e is zero.
func (e *nodeExt) empty() bool {
	return e.Expression == nil && e.Body == nil && e.Condition == nil && e.Then == nil && e.Else == nil &&
		e.Pattern == nil && e.Update == nil && e.Delete == nil && e.LHS == nil && e.Terms == nil &&
		e.Signature == nil && e.Focus == "" && e.Index == "" && e.Group == nil
}

// x returns the rare fields of n, allocating them on first write.
func (n *Node) x() *nodeExt {
	if n.ext == nil {
		n.ext = &nodeExt{}
	}
	return n.ext
}

// Expression is the operand of unary minus.
func (n *Node) Expression() *Node {
	if n.ext == nil {
		return nil
	}
	return n.ext.Expression
}

// Body is the body of a lambda.
func (n *Node) Body() *Node {
	if n.ext == nil {
		return nil
	}
	return n.ext.Body
}

// Condition is the test of a condition node.
func (n *Node) Condition() *Node {
	if n.ext == nil {
		return nil
	}
	return n.ext.Condition
}

// Then is the branch of a condition node taken when the test is true.
func (n *Node) Then() *Node {
	if n.ext == nil {
		return nil
	}
	return n.ext.Then
}

// Else is the branch of a condition node taken when the test is false; nil
// if there is no else branch.
func (n *Node) Else() *Node {
	if n.ext == nil {
		return nil
	}
	return n.ext.Else
}

// Pattern is the pattern of a transform.
func (n *Node) Pattern() *Node {
	if n.ext == nil {
		return nil
	}
	return n.ext.Pattern
}

// Update is the update of a transform.
func (n *Node) Update() *Node {
	if n.ext == nil {
		return nil
	}
	return n.ext.Update
}

// Delete is the delete clause of a transform; nil if absent.
func (n *Node) Delete() *Node {
	if n.ext == nil {
		return nil
	}
	return n.ext.Delete
}

// LHS holds the key-value pairs of an object constructor, flat
// [k0,v0,k1,v1,...].
func (n *Node) LHS() []*Node {
	if n.ext == nil {
		return nil
	}
	return n.ext.LHS
}

// Terms are the terms of a sort.
func (n *Node) Terms() []SortTerm {
	if n.ext == nil {
		return nil
	}
	return n.ext.Terms
}

// Signature is the parsed signature of a lambda; nil if absent.
func (n *Node) Signature() *Signature {
	if n.ext == nil {
		return nil
	}
	return n.ext.Signature
}

// Focus is the variable name bound by @.
func (n *Node) Focus() string {
	if n.ext == nil {
		return ""
	}
	return n.ext.Focus
}

// Index is the variable name bound by #.
func (n *Node) Index() string {
	if n.ext == nil {
		return ""
	}
	return n.ext.Index
}

// Group is the group expression attached to a path or step.
func (n *Node) Group() *GroupExpr {
	if n.ext == nil {
		return nil
	}
	return n.ext.Group
}

// WithoutGroup returns a copy of n without its group expression, sharing
// everything else. The tree is shared read-only, so n is not modified.
func (n *Node) WithoutGroup() *Node {
	cp := *n
	if n.ext != nil {
		ext := *n.ext
		ext.Group = nil
		if ext.empty() {
			cp.ext = nil
		} else {
			cp.ext = &ext
		}
	}
	return &cp
}

// SortTerm is one term in a sort expression.
type SortTerm struct {
	Descending bool
	Expression *Node
}

// GroupExpr holds the key-value pairs of a group expression {}.
type GroupExpr struct {
	Pairs [][2]*Node // [key-expr, value-expr] pairs
	Pos   int
}

// Signature is the parsed type signature of a lambda or built-in function.
// The full parsing logic lives in functions/signature.go; this type is shared.
type Signature struct {
	Raw string // original signature string e.g. "<s-n?:s>"
}

// Node type constants.
const (
	NodePath NodeType = iota + 1
	NodeBinary
	NodeUnary
	NodeName
	NodeString
	NodeNumber
	NodeValue
	NodeVariable
	NodeWildcard
	NodeDescendant
	NodeParent
	NodeRegex
	NodeBlock
	NodeCondition
	NodeBind
	NodeFunction
	NodePartial
	NodeLambda
	NodeApply
	NodeTransform
	NodeSort
	NodePlaceholder // placeholder "?" in partial application
)

var nodeTypeNames = [...]string{
	NodePath:        "path",
	NodeBinary:      "binary",
	NodeUnary:       "unary",
	NodeName:        "name",
	NodeString:      "string",
	NodeNumber:      "number",
	NodeValue:       "value",
	NodeVariable:    "variable",
	NodeWildcard:    "wildcard",
	NodeDescendant:  "descendant",
	NodeParent:      "parent",
	NodeRegex:       "regex",
	NodeBlock:       "block",
	NodeCondition:   "condition",
	NodeBind:        "bind",
	NodeFunction:    "function",
	NodePartial:     "partial",
	NodeLambda:      "lambda",
	NodeApply:       "apply",
	NodeTransform:   "transform",
	NodeSort:        "sort",
	NodePlaceholder: "operator",
}

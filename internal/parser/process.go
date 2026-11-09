package parser

// ProcessAST runs the post-processing pass over the raw Pratt-parsed tree,
// transforming it into a form suitable for evaluation.
//
// Current transformations:
//   - Flattens nested binary(".") nodes into path nodes with Steps slices.
//   - Propagates KeepSingletonArray when any step has KeepArray=true.
//   - Marks last-step array constructors as ConsArray (jsonata-js consarray).
//   - Attaches group expressions from path-step binary("{") to the path.
//   - Recursively processes all child nodes.
func ProcessAST(node *Node) (*Node, error) {
	if node == nil {
		return nil, nil
	}

	switch node.Type {
	case NodeBinary:
		if node.Value == "." {
			return processDotBinary(node)
		}
		return processBinaryChildren(node)

	case NodeUnary:
		return processUnaryChildren(node)

	case NodeBlock:
		return processBlockChildren(node)

	case NodeFunction, NodePartial:
		return processFunctionChildren(node)

	case NodeLambda:
		return processLambdaChildren(node)

	case NodeCondition:
		return processConditionChildren(node)

	case NodeBind:
		return processBindChildren(node)

	case NodeTransform:
		return processTransformChildren(node)

	case NodeSort:
		return processSortChildren(node)

	case NodePath:
		return processPathChildren(node)

	default:
		// Leaf nodes (name, string, number, value, variable, wildcard, descendant, parent, regex).
		// Still need to process any attached Group expression.
		if err := processGroupPairs(node.Group()); err != nil {
			return nil, err
		}
		return node, nil
	}
}

// processDotBinary flattens a binary(".") node into a path node, in place:
// the binary node becomes the path of the steps of its operands.
func processDotBinary(node *Node) (*Node, error) {
	// Collect all steps from nested dots.
	steps, err := appendPathSteps(nil, node)
	if err != nil {
		return nil, err
	}

	// propagate group-by expression (A.B{key:val})
	flat := Node{Type: NodePath, Value: node.Value, Steps: steps, Pos: node.Pos}
	if group := node.Group(); group != nil {
		flat.ext = &nodeExt{Group: group}
	}
	*node = flat
	path := node
	markUnaryArraySteps(steps)

	// Propagate KeepSingletonArray when any step (or a subscript step's left side)
	// has KeepArray=true. This covers both A[].B and A[][filter].B patterns.
	for _, s := range steps {
		if s.KeepArray {
			path.KeepSingletonArray = true
			break
		}
		// A[][filter] — the KeepArray flag is on the Name/Var left of the subscript.
		if s.Type == NodeBinary && s.Value == "[" && s.Left != nil && s.Left.KeepArray {
			path.KeepSingletonArray = true
			break
		}
	}

	// Process group-by key/value pairs so nested dot expressions within them are resolved.
	if err := processGroupPairs(path.Group()); err != nil {
		return nil, err
	}

	return path, nil
}

// appendPathSteps recursively appends to steps the steps of binary(".")
// nodes, in one slice for the whole path.
func appendPathSteps(steps []*Node, node *Node) ([]*Node, error) {
	if node.Type != NodeBinary || node.Value != "." {
		// Leaf step — process it.
		processed, err := ProcessAST(node)
		if err != nil {
			return nil, err
		}
		// If the processed node is itself a path, splice its steps.
		if processed.Type == NodePath {
			return append(steps, processed.Steps...), nil
		}
		promoteQuotedPathNames(processed)
		if processed.Type == NodeString {
			*processed = Node{Type: NodeName, Value: processed.Value, Pos: processed.Pos}
		}
		return append(steps, processed), nil
	}

	start := len(steps)
	steps, err := appendPathSteps(steps, node.Left)
	if err != nil {
		return nil, err
	}
	lastLeft := len(steps) - 1
	steps, err = appendPathSteps(steps, node.Right)
	if err != nil {
		return nil, err
	}

	// The # and @ infix operators set Index/Focus on the binary "." node
	// itself (e.g. (Account.Order)#$o). Propagate these to the last left
	// step so the binding survives path flattening.
	if lastLeft >= start {
		last := steps[lastLeft]
		if index := node.Index(); index != "" {
			last.x().Index = index
		}
		if focus := node.Focus(); focus != "" {
			last.x().Focus = focus
		}
	}

	return steps, nil
}

func promoteQuotedPathNames(n *Node) {
	for n != nil && n.Type == NodeBinary && n.Value == "[" {
		if n.Left != nil && n.Left.Type == NodeString {
			left := n.Left
			name := Node{
				Type:      NodeName,
				Value:     left.Value,
				Pos:       left.Pos,
				KeepArray: left.KeepArray,
			}
			if group, index, focus := left.Group(), left.Index(), left.Focus(); group != nil || index != "" || focus != "" {
				name.ext = &nodeExt{Group: group, Index: index, Focus: focus}
			}
			*left = name
			return
		}
		n = n.Left
	}
}

// processBinaryChildren recursively processes a non-dot binary node.
func processBinaryChildren(node *Node) (*Node, error) {
	var err error
	node.Left, err = ProcessAST(node.Left)
	if err != nil {
		return nil, err
	}
	node.Right, err = ProcessAST(node.Right)
	if err != nil {
		return nil, err
	}
	return node, nil
}

// processUnaryChildren recursively processes a unary node.
func processUnaryChildren(node *Node) (*Node, error) {
	var err error
	if node.Expression() != nil {
		node.ext.Expression, err = ProcessAST(node.ext.Expression)
		if err != nil {
			return nil, err
		}
	}
	for i, expr := range node.Expressions {
		node.Expressions[i], err = ProcessAST(expr)
		if err != nil {
			return nil, err
		}
	}
	lhs := node.LHS()
	for i, n := range lhs {
		lhs[i], err = ProcessAST(n)
		if err != nil {
			return nil, err
		}
	}
	return node, nil
}

// processBlockChildren recursively processes a block node.
func processBlockChildren(node *Node) (*Node, error) {
	var err error
	for i, expr := range node.Expressions {
		node.Expressions[i], err = ProcessAST(expr)
		if err != nil {
			return nil, err
		}
		part := node.Expressions[i]
		if part.ConsArray || (part.Type == NodePath && len(part.Steps) > 0 && part.Steps[0].ConsArray) {
			node.ConsArray = true
		}
	}
	return node, nil
}

// markUnaryArraySteps flags first/last path steps that are array constructors
// so evaluation can treat them as cons arrays (jsonata-js processAST).
func markUnaryArraySteps(steps []*Node) {
	if len(steps) == 0 {
		return
	}
	last := steps[len(steps)-1]
	if isUnaryArrayCtor(last) {
		last.ConsArray = true
	}
}

func isUnaryArrayCtor(n *Node) bool {
	return n != nil && n.Type == NodeUnary && n.Value == "["
}

// processFunctionChildren recursively processes a function/partial node.
func processFunctionChildren(node *Node) (*Node, error) {
	var err error
	node.Procedure, err = ProcessAST(node.Procedure)
	if err != nil {
		return nil, err
	}
	for i, arg := range node.Arguments {
		node.Arguments[i], err = ProcessAST(arg)
		if err != nil {
			return nil, err
		}
	}
	return node, nil
}

// processLambdaChildren recursively processes a lambda node.
func processLambdaChildren(node *Node) (*Node, error) {
	body, err := ProcessAST(node.Body())
	if err != nil {
		return nil, err
	}
	if body != nil {
		node.x().Body = body
	}
	markTailCalls(body)
	return node, nil
}

// processConditionChildren recursively processes a condition node.
func processConditionChildren(node *Node) (*Node, error) {
	ext := node.x()
	var err error
	ext.Condition, err = ProcessAST(ext.Condition)
	if err != nil {
		return nil, err
	}
	ext.Then, err = ProcessAST(ext.Then)
	if err != nil {
		return nil, err
	}
	if ext.Else != nil {
		ext.Else, err = ProcessAST(ext.Else)
		if err != nil {
			return nil, err
		}
	}
	return node, nil
}

// processBindChildren recursively processes a bind node.
func processBindChildren(node *Node) (*Node, error) {
	var err error
	node.Left, err = ProcessAST(node.Left)
	if err != nil {
		return nil, err
	}
	node.Right, err = ProcessAST(node.Right)
	if err != nil {
		return nil, err
	}
	return node, nil
}

// processTransformChildren recursively processes a transform node.
func processTransformChildren(node *Node) (*Node, error) {
	ext := node.x()
	var err error
	ext.Pattern, err = ProcessAST(ext.Pattern)
	if err != nil {
		return nil, err
	}
	ext.Update, err = ProcessAST(ext.Update)
	if err != nil {
		return nil, err
	}
	if ext.Delete != nil {
		ext.Delete, err = ProcessAST(ext.Delete)
		if err != nil {
			return nil, err
		}
	}
	return node, nil
}

// processSortChildren recursively processes a sort node.
func processSortChildren(node *Node) (*Node, error) {
	var err error
	node.Left, err = ProcessAST(node.Left)
	if err != nil {
		return nil, err
	}
	terms := node.Terms()
	for i, term := range terms {
		terms[i].Expression, err = ProcessAST(term.Expression)
		if err != nil {
			return nil, err
		}
	}
	return node, nil
}

// processPathChildren recursively processes an existing path node.
func processPathChildren(node *Node) (*Node, error) {
	var err error
	for i, step := range node.Steps {
		node.Steps[i], err = ProcessAST(step)
		if err != nil {
			return nil, err
		}
		if node.Steps[i].KeepArray {
			node.KeepSingletonArray = true
		}
	}
	markUnaryArraySteps(node.Steps)
	// Process group-by key/value pairs.
	if err := processGroupPairs(node.Group()); err != nil {
		return nil, err
	}
	return node, nil
}

// processGroupPairs processes the key/value pairs of a group-by expression,
// if any.
func processGroupPairs(group *GroupExpr) error {
	if group == nil {
		return nil
	}
	var err error
	for i, pair := range group.Pairs {
		group.Pairs[i][0], err = ProcessAST(pair[0])
		if err != nil {
			return err
		}
		group.Pairs[i][1], err = ProcessAST(pair[1])
		if err != nil {
			return err
		}
	}
	return nil
}

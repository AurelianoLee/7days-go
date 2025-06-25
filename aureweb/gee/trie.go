package gee

import "strings"

type node struct {
	pattern  string  // 待匹配的路由，例如 /p/:lang
	part     string  // 路由中的一部分，例如 :lang
	children []*node // 子节点，例如 [doc, tutorial, intro]
	isWild   bool    // 是否精确匹配，part 含有 : 或 * 时为true
}

// 第一个匹配成功的节点，用于插入
//
// used in r.GET("/:lang/doc", func(c *gee.Context) {})
func (n *node) matchChild(part string) *node {
	// 如果当前节点的part与part相等，或者当前节点的isWild为true，则返回当前节点
	// 否则，继续遍历当前节点的子节点
	// 如果遍历完所有子节点，则返回nil
	for _, child := range n.children {
		if child.part == part || child.isWild {
			return child
		}
	}
	return nil
}

// 所有匹配成功的节点，用于查找
func (n *node) matchChildren(part string) []*node {
	// 详细解释原理
	// 如果当前节点的part与part相等，或者当前节点的isWild为true，则将当前节点添加到nodes中
	// 否则，继续遍历当前节点的子节点
	// 如果遍历完所有子节点，则返回nodes
	nodes := make([]*node, 0)
	for _, child := range n.children {
		if child.part == part || child.isWild {
			nodes = append(nodes, child)
		}
	}
	return nodes
}

func (n *node) insert(pattern string, parts []string, height int) {
	if len(parts) == height {
		n.pattern = pattern
		return
	}
	part := parts[height]
	child := n.matchChild(part)
	// 如果当前节点没有匹配到part，则新建一个节点
	if child == nil {
		// 如果当前的part是:或者*，则设置isWild为true
		child = &node{part: part, isWild: part[0] == ':' || part[0] == '*'}
		n.children = append(n.children, child)
	}
	child.insert(pattern, parts, height+1)
}

func (n *node) search(parts []string, height int) *node {
	if len(parts) == height || strings.HasPrefix(n.part, "*") {
		if n.pattern == "" {
			return nil
		}
		return n
	}

	// 指名当前要匹配的part
	part := parts[height]
	// 获取所有匹配的子节点
	children := n.matchChildren(part)

	// 遍历所有匹配的子节点
	for _, child := range children {
		// 递归查找下一层节点
		result := child.search(parts, height+1)
		if result != nil {
			return result
		}
	}
	// 如果遍历完所有子节点，且没有匹配到，则返回nil
	return nil
}

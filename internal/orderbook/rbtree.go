package orderbook

import (
	"github.com/shopspring/decimal"
)

// RBTree 红黑树实现
type RBTree struct {
	root    *RBNode
	size    int
	desc    bool // 是否降序
	nilNode *RBNode
}

// RBNode 红黑树节点
type RBNode struct {
	key    decimal.Decimal
	value  *PriceLevel
	color  bool // true=red, false=black
	left   *RBNode
	right  *RBNode
	parent *RBNode
}

const (
	red   = true
	black = false
)

// NewRBTree 创建红黑树
func NewRBTree(desc bool) *RBTree {
	nilNode := &RBNode{color: black}
	nilNode.left = nilNode
	nilNode.right = nilNode
	nilNode.parent = nilNode
	return &RBTree{
		root:    nilNode,
		nilNode: nilNode,
		desc:    desc,
	}
}

// Insert 插入节点
func (t *RBTree) Insert(key decimal.Decimal, value *PriceLevel) {
	node := &RBNode{
		key:    key,
		value:  value,
		color:  red,
		left:   t.nilNode,
		right:  t.nilNode,
		parent: t.nilNode,
	}

	parent := t.nilNode
	current := t.root

	for current != t.nilNode {
		parent = current
		cmp := key.Cmp(current.key)
		if cmp == 0 {
			// 键已存在，更新值
			current.value = value
			return
		}
		if t.compare(cmp) < 0 {
			current = current.left
		} else {
			current = current.right
		}
	}

	node.parent = parent
	if parent == t.nilNode {
		t.root = node
	} else {
		cmp := key.Cmp(parent.key)
		if t.compare(cmp) < 0 {
			parent.left = node
		} else {
			parent.right = node
		}
	}

	t.size++
	t.insertFixup(node)
}

// compare 根据排序方向调整比较结果
func (t *RBTree) compare(cmp int) int {
	if t.desc {
		return -cmp
	}
	return cmp
}

// insertFixup 插入修复
func (t *RBTree) insertFixup(node *RBNode) {
	for node.parent.color == red {
		if node.parent == node.parent.parent.left {
			uncle := node.parent.parent.right
			if uncle.color == red {
				node.parent.color = black
				uncle.color = black
				node.parent.parent.color = red
				node = node.parent.parent
			} else {
				if node == node.parent.right {
					node = node.parent
					t.leftRotate(node)
				}
				node.parent.color = black
				node.parent.parent.color = red
				t.rightRotate(node.parent.parent)
			}
		} else {
			uncle := node.parent.parent.left
			if uncle.color == red {
				node.parent.color = black
				uncle.color = black
				node.parent.parent.color = red
				node = node.parent.parent
			} else {
				if node == node.parent.left {
					node = node.parent
					t.rightRotate(node)
				}
				node.parent.color = black
				node.parent.parent.color = red
				t.leftRotate(node.parent.parent)
			}
		}
		if node == t.root {
			break
		}
	}
	t.root.color = black
}

// leftRotate 左旋
func (t *RBTree) leftRotate(x *RBNode) {
	y := x.right
	x.right = y.left
	if y.left != t.nilNode {
		y.left.parent = x
	}
	y.parent = x.parent
	if x.parent == t.nilNode {
		t.root = y
	} else if x == x.parent.left {
		x.parent.left = y
	} else {
		x.parent.right = y
	}
	y.left = x
	x.parent = y
}

// rightRotate 右旋
func (t *RBTree) rightRotate(x *RBNode) {
	y := x.left
	x.left = y.right
	if y.right != t.nilNode {
		y.right.parent = x
	}
	y.parent = x.parent
	if x.parent == t.nilNode {
		t.root = y
	} else if x == x.parent.right {
		x.parent.right = y
	} else {
		x.parent.left = y
	}
	y.right = x
	x.parent = y
}

// Get 获取节点
func (t *RBTree) Get(key decimal.Decimal) *PriceLevel {
	node := t.search(key)
	if node == t.nilNode {
		return nil
	}
	return node.value
}

// search 搜索节点
func (t *RBTree) search(key decimal.Decimal) *RBNode {
	current := t.root
	for current != t.nilNode {
		cmp := key.Cmp(current.key)
		if cmp == 0 {
			return current
		}
		if t.compare(cmp) < 0 {
			current = current.left
		} else {
			current = current.right
		}
	}
	return t.nilNode
}

// Delete 删除节点
func (t *RBTree) Delete(key decimal.Decimal) {
	node := t.search(key)
	if node == t.nilNode {
		return
	}
	t.deleteNode(node)
}

// deleteNode 删除节点
func (t *RBTree) deleteNode(z *RBNode) {
	var x, y *RBNode
	y = z
	yOriginalColor := y.color

	if z.left == t.nilNode {
		x = z.right
		t.transplant(z, z.right)
	} else if z.right == t.nilNode {
		x = z.left
		t.transplant(z, z.left)
	} else {
		y = t.minimum(z.right)
		yOriginalColor = y.color
		x = y.right
		if y.parent == z {
			x.parent = y
		} else {
			t.transplant(y, y.right)
			y.right = z.right
			y.right.parent = y
		}
		t.transplant(z, y)
		y.left = z.left
		y.left.parent = y
		y.color = z.color
	}

	t.size--
	if yOriginalColor == black {
		t.deleteFixup(x)
	}
}

// transplant 移植
func (t *RBTree) transplant(u, v *RBNode) {
	if u.parent == t.nilNode {
		t.root = v
	} else if u == u.parent.left {
		u.parent.left = v
	} else {
		u.parent.right = v
	}
	v.parent = u.parent
}

// minimum 最小节点
func (t *RBTree) minimum(node *RBNode) *RBNode {
	for node.left != t.nilNode {
		node = node.left
	}
	return node
}

// maximum 最大节点
func (t *RBTree) maximum(node *RBNode) *RBNode {
	for node.right != t.nilNode {
		node = node.right
	}
	return node
}

// deleteFixup 删除修复
func (t *RBTree) deleteFixup(x *RBNode) {
	for x != t.root && x.color == black {
		if x == x.parent.left {
			w := x.parent.right
			if w.color == red {
				w.color = black
				x.parent.color = red
				t.leftRotate(x.parent)
				w = x.parent.right
			}
			if w.left.color == black && w.right.color == black {
				w.color = red
				x = x.parent
			} else {
				if w.right.color == black {
					w.left.color = black
					w.color = red
					t.rightRotate(w)
					w = x.parent.right
				}
				w.color = x.parent.color
				x.parent.color = black
				w.right.color = black
				t.leftRotate(x.parent)
				x = t.root
			}
		} else {
			w := x.parent.left
			if w.color == red {
				w.color = black
				x.parent.color = red
				t.rightRotate(x.parent)
				w = x.parent.left
			}
			if w.right.color == black && w.left.color == black {
				w.color = red
				x = x.parent
			} else {
				if w.left.color == black {
					w.right.color = black
					w.color = red
					t.leftRotate(w)
					w = x.parent.left
				}
				w.color = x.parent.color
				x.parent.color = black
				w.left.color = black
				t.rightRotate(x.parent)
				x = t.root
			}
		}
	}
	x.color = black
}

// Min 获取最小值
func (t *RBTree) Min() *PriceLevel {
	if t.root == t.nilNode {
		return nil
	}
	node := t.minimum(t.root)
	return node.value
}

// Max 获取最大值
func (t *RBTree) Max() *PriceLevel {
	if t.root == t.nilNode {
		return nil
	}
	node := t.maximum(t.root)
	return node.value
}

// Size 获取大小
func (t *RBTree) Size() int {
	return t.size
}

// Ascend 升序遍历
func (t *RBTree) Ascend(callback func(decimal.Decimal, *PriceLevel) bool) {
	t.ascend(t.root, callback)
}

// ascend 递归遍历
func (t *RBTree) ascend(node *RBNode, callback func(decimal.Decimal, *PriceLevel) bool) bool {
	if node == t.nilNode {
		return true
	}
	if !t.ascend(node.left, callback) {
		return false
	}
	if !callback(node.key, node.value) {
		return false
	}
	return t.ascend(node.right, callback)
}

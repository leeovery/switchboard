package views

import (
	"iter"
	"slices"
	"sort"
)

// chunkRows is how many items a chunk of a chunked list holds once it's
// built whole, and twice it the most a chunk grows to before it's split.
const chunkRows = 256

// chunked is a list held in chunks never changed once made: a change makes a
// list of its own, copying the chunk it changes and the list of chunks, and
// sharing every other chunk with the list it was made from, so a change
// costs a chunk, never the items of the others, and a holder of either
// never sees it change. Copied counts the chunks its changes copied, from
// the first list it was made from on.
type chunked[T any] struct {
	chunks [][]T
	// starts are where each chunk starts among the items.
	starts []int
	n      int
	copied int
}

// chunkedOf is a list of items, in chunks of chunkRows.
func chunkedOf[T any](items []T) chunked[T] {
	var c chunked[T]
	for from := 0; from < len(items); from += chunkRows {
		c.chunks = append(c.chunks, slices.Clone(items[from:min(from+chunkRows, len(items))]))
	}
	c.restart(0)
	return c
}

// len is how many items c holds.
func (c chunked[T]) len() int {
	return c.n
}

// at is the item at the place given.
func (c chunked[T]) at(i int) T {
	k, j := c.place(i)
	return c.chunks[k][j]
}

// place is the chunk the item at the place given is in, and its place in
// it: past the last chunk's last item for the place after the last.
func (c chunked[T]) place(i int) (chunk, at int) {
	k := sort.Search(len(c.starts), func(k int) bool { return c.starts[k] > i }) - 1
	if k < 0 {
		return 0, 0
	}
	return k, i - c.starts[k]
}

// search is the first place whose item reports true, as sort.Search has it,
// the items reporting false before it and true from it on: the place after
// the last where none does.
func (c chunked[T]) search(from func(T) bool) int {
	k := sort.Search(len(c.chunks), func(k int) bool { return from(c.chunks[k][len(c.chunks[k])-1]) })
	if k == len(c.chunks) {
		return c.n
	}
	return c.starts[k] + sort.Search(len(c.chunks[k]), func(j int) bool { return from(c.chunks[k][j]) })
}

// all are the items, in order, with their places.
func (c chunked[T]) all() iter.Seq2[int, T] {
	return func(yield func(int, T) bool) {
		for k, chunk := range c.chunks {
			for j, item := range chunk {
				if !yield(c.starts[k]+j, item) {
					return
				}
			}
		}
	}
}

// from are the items from the place given on, in order, with their places.
func (c chunked[T]) from(i int) iter.Seq2[int, T] {
	return func(yield func(int, T) bool) {
		if i >= c.n {
			return
		}
		k, j := c.place(i)
		for ; k < len(c.chunks); k, j = k+1, 0 {
			for ; j < len(c.chunks[k]); j++ {
				if !yield(c.starts[k]+j, c.chunks[k][j]) {
					return
				}
			}
		}
	}
}

// inserted is c with the item put at the place given, those from it on
// after it: into the chunk it falls in, split in two where it grows past
// twice chunkRows.
func (c chunked[T]) inserted(i int, item T) chunked[T] {
	if c.n == 0 {
		c.chunks, c.starts, c.n = [][]T{{item}}, []int{0}, 1
		c.copied++
		return c
	}
	k, j := c.place(min(i, c.n-1))
	if i == c.n {
		k, j = len(c.chunks)-1, len(c.chunks[len(c.chunks)-1])
	}
	grown := slices.Insert(slices.Clone(c.chunks[k]), j, item)
	c.chunks = slices.Clone(c.chunks)
	c.copied++
	if len(grown) <= 2*chunkRows {
		c.chunks[k] = grown
	} else {
		half := len(grown) / 2
		c.chunks = slices.Insert(c.chunks, k+1, grown[half:])
		c.chunks[k] = grown[:half:half]
	}
	c.n++
	c.restart(k)
	return c
}

// appended is c with the items put after its last, in order: its last chunk
// copied once, filled to chunkRows, and the rest in chunks of their own, as
// chunkedOf has them, so a day built whole copies a chunk for each
// chunkRows of its items, never one for each item.
func (c chunked[T]) appended(items []T) chunked[T] {
	if len(items) == 0 {
		return c
	}
	c.chunks = slices.Clone(c.chunks)
	from := len(c.chunks)
	if last := from - 1; last >= 0 && len(c.chunks[last]) < chunkRows {
		filled := min(chunkRows-len(c.chunks[last]), len(items))
		c.chunks[last] = slices.Concat(c.chunks[last], items[:filled])
		items, from = items[filled:], last
		c.copied++
	}
	rest := chunkedOf(items)
	c.chunks = append(c.chunks, rest.chunks...)
	c.copied += len(rest.chunks)
	c.restart(from)
	return c
}

// set is c with the item at the place given in place of the one there.
func (c chunked[T]) set(i int, item T) chunked[T] {
	k, j := c.place(i)
	c.chunks = slices.Clone(c.chunks)
	c.chunks[k] = slices.Clone(c.chunks[k])
	c.chunks[k][j] = item
	c.copied++
	return c
}

// restart works out where each chunk from the one given on starts, the
// list of them c's own, and how many items c holds.
func (c *chunked[T]) restart(from int) {
	starts := slices.Clone(c.starts[:min(from, len(c.starts))])
	n := 0
	if from > 0 {
		n = starts[from-1] + len(c.chunks[from-1])
	}
	for _, chunk := range c.chunks[from:] {
		starts = append(starts, n)
		n += len(chunk)
	}
	c.starts, c.n = starts, n
}

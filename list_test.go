package dagro

import "testing"

func TestFASListUpstreamDequeue(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if got := (&fasList{}).dequeue(); got != nil {
			t.Fatalf("dequeue() = %#v, want nil", got)
		}
	})

	t.Run("one entry", func(t *testing.T) {
		list := &fasList{}
		entry := &fasEntry{V: "entry"}
		list.enqueue(entry)
		if got := list.dequeue(); got != entry {
			t.Fatalf("dequeue() = %p, want %p", got, entry)
		}
		if entry.prev != nil || entry.next != nil || entry.currentList != nil {
			t.Fatalf("dequeued entry retained list links: %#v", entry)
		}
	})

	t.Run("FIFO", func(t *testing.T) {
		list := &fasList{}
		first, second := &fasEntry{V: "first"}, &fasEntry{V: "second"}
		list.enqueue(first)
		list.enqueue(second)
		if got := list.dequeue(); got != first {
			t.Fatalf("first dequeue = %p, want %p", got, first)
		}
		if got := list.dequeue(); got != second {
			t.Fatalf("second dequeue = %p, want %p", got, second)
		}
	})

	t.Run("re-enqueue moves entry to newest position", func(t *testing.T) {
		list := &fasList{}
		first, second := &fasEntry{V: "first"}, &fasEntry{V: "second"}
		list.enqueue(first)
		list.enqueue(second)
		list.enqueue(first)
		if got := list.dequeue(); got != second {
			t.Fatalf("first dequeue = %p, want %p", got, second)
		}
		if got := list.dequeue(); got != first {
			t.Fatalf("second dequeue = %p, want %p", got, first)
		}
	})

	t.Run("enqueue on another list transfers ownership", func(t *testing.T) {
		first, second := &fasList{}, &fasList{}
		entry := &fasEntry{V: "entry"}
		first.enqueue(entry)
		second.enqueue(entry)
		if got := first.dequeue(); got != nil {
			t.Fatalf("old list dequeue = %#v, want nil", got)
		}
		if got := second.dequeue(); got != entry {
			t.Fatalf("new list dequeue = %p, want %p", got, entry)
		}
	})
}

func TestFASListUpstreamString(t *testing.T) {
	list := &fasList{}
	list.enqueue(&fasEntry{data: map[string]any{"entry": 1}})
	list.enqueue(&fasEntry{data: map[string]any{"entry": 2}})
	if got, want := list.String(), `[{"entry":1}, {"entry":2}]`; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

package xetcd

import (
	"context"
	"testing"
)

// 以下测试验证参数校验逻辑，不需要真实的 etcd 连接

func TestKV_Get_Closed(t *testing.T) {
	c := &Client{}
	c.closed.Store(true)

	_, err := c.Get(context.Background(), "key")
	if err != ErrClientClosed {
		t.Errorf("Get() on closed client = %v, want %v", err, ErrClientClosed)
	}
}

func TestKV_Get_EmptyKey(t *testing.T) {
	c := &Client{}

	_, err := c.Get(context.Background(), "")
	if err != ErrEmptyKey {
		t.Errorf("Get() with empty key = %v, want %v", err, ErrEmptyKey)
	}
}

func TestKV_GetWithRevision_Closed(t *testing.T) {
	c := &Client{}
	c.closed.Store(true)

	_, _, err := c.GetWithRevision(context.Background(), "key")
	if err != ErrClientClosed {
		t.Errorf("GetWithRevision() on closed client = %v, want %v", err, ErrClientClosed)
	}
}

func TestKV_GetWithRevision_EmptyKey(t *testing.T) {
	c := &Client{}

	_, _, err := c.GetWithRevision(context.Background(), "")
	if err != ErrEmptyKey {
		t.Errorf("GetWithRevision() with empty key = %v, want %v", err, ErrEmptyKey)
	}
}

func TestKV_Put_Closed(t *testing.T) {
	c := &Client{}
	c.closed.Store(true)

	err := c.Put(context.Background(), "key", []byte("value"))
	if err != ErrClientClosed {
		t.Errorf("Put() on closed client = %v, want %v", err, ErrClientClosed)
	}
}

func TestKV_Put_EmptyKey(t *testing.T) {
	c := &Client{}

	err := c.Put(context.Background(), "", []byte("value"))
	if err != ErrEmptyKey {
		t.Errorf("Put() with empty key = %v, want %v", err, ErrEmptyKey)
	}
}

func TestKV_PutWithTTL_Closed(t *testing.T) {
	c := &Client{}
	c.closed.Store(true)

	err := c.PutWithTTL(context.Background(), "key", []byte("value"), 10)
	if err != ErrClientClosed {
		t.Errorf("PutWithTTL() on closed client = %v, want %v", err, ErrClientClosed)
	}
}

func TestKV_PutWithTTL_EmptyKey(t *testing.T) {
	c := &Client{}

	err := c.PutWithTTL(context.Background(), "", []byte("value"), 10)
	if err != ErrEmptyKey {
		t.Errorf("PutWithTTL() with empty key = %v, want %v", err, ErrEmptyKey)
	}
}

func TestKV_Delete_Closed(t *testing.T) {
	c := &Client{}
	c.closed.Store(true)

	err := c.Delete(context.Background(), "key")
	if err != ErrClientClosed {
		t.Errorf("Delete() on closed client = %v, want %v", err, ErrClientClosed)
	}
}

func TestKV_Delete_EmptyKey(t *testing.T) {
	c := &Client{}

	err := c.Delete(context.Background(), "")
	if err != ErrEmptyKey {
		t.Errorf("Delete() with empty key = %v, want %v", err, ErrEmptyKey)
	}
}

func TestKV_DeleteWithPrefix_Closed(t *testing.T) {
	c := &Client{}
	c.closed.Store(true)

	_, err := c.DeleteWithPrefix(context.Background(), "/prefix/")
	if err != ErrClientClosed {
		t.Errorf("DeleteWithPrefix() on closed client = %v, want %v", err, ErrClientClosed)
	}
}

func TestKV_DeleteWithPrefix_EmptyKey(t *testing.T) {
	c := &Client{}

	_, err := c.DeleteWithPrefix(context.Background(), "")
	if err != ErrEmptyKey {
		t.Errorf("DeleteWithPrefix() with empty key = %v, want %v", err, ErrEmptyKey)
	}
}

func TestKV_List_Closed(t *testing.T) {
	c := &Client{}
	c.closed.Store(true)

	_, err := c.List(context.Background(), "/prefix/")
	if err != ErrClientClosed {
		t.Errorf("List() on closed client = %v, want %v", err, ErrClientClosed)
	}
}

func TestKV_List_EmptyKey(t *testing.T) {
	c := &Client{}

	_, err := c.List(context.Background(), "")
	if err != ErrEmptyKey {
		t.Errorf("List() with empty key = %v, want %v", err, ErrEmptyKey)
	}
}

func TestKV_ListKeys_Closed(t *testing.T) {
	c := &Client{}
	c.closed.Store(true)

	_, err := c.ListKeys(context.Background(), "/prefix/")
	if err != ErrClientClosed {
		t.Errorf("ListKeys() on closed client = %v, want %v", err, ErrClientClosed)
	}
}

func TestKV_ListKeys_EmptyKey(t *testing.T) {
	c := &Client{}

	_, err := c.ListKeys(context.Background(), "")
	if err != ErrEmptyKey {
		t.Errorf("ListKeys() with empty key = %v, want %v", err, ErrEmptyKey)
	}
}

func TestKV_Exists_Closed(t *testing.T) {
	c := &Client{}
	c.closed.Store(true)

	_, err := c.Exists(context.Background(), "key")
	if err != ErrClientClosed {
		t.Errorf("Exists() on closed client = %v, want %v", err, ErrClientClosed)
	}
}

func TestKV_Exists_EmptyKey(t *testing.T) {
	c := &Client{}

	_, err := c.Exists(context.Background(), "")
	if err != ErrEmptyKey {
		t.Errorf("Exists() with empty key = %v, want %v", err, ErrEmptyKey)
	}
}

func TestKV_Count_Closed(t *testing.T) {
	c := &Client{}
	c.closed.Store(true)

	_, err := c.Count(context.Background(), "/prefix/")
	if err != ErrClientClosed {
		t.Errorf("Count() on closed client = %v, want %v", err, ErrClientClosed)
	}
}

func TestKV_Count_EmptyKey(t *testing.T) {
	c := &Client{}

	_, err := c.Count(context.Background(), "")
	if err != ErrEmptyKey {
		t.Errorf("Count() with empty key = %v, want %v", err, ErrEmptyKey)
	}
}

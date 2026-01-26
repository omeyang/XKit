package xdbg

import (
	"os/user"
	"strings"
	"testing"
)

func TestResolveIdentity(t *testing.T) {
	// 获取当前用户信息用于测试
	currentUser, err := user.Current()
	if err != nil {
		t.Skip("cannot get current user")
	}

	peer := &PeerIdentity{
		UID: 0,
		GID: 0,
		PID: 12345,
	}

	// 使用当前用户的 UID（仅作为日志记录）
	t.Logf("current uid: %s", currentUser.Uid)

	info := ResolveIdentity(peer)

	if info.PeerIdentity != peer {
		t.Error("PeerIdentity should be embedded")
	}

	if info.PID != 12345 {
		t.Errorf("PID = %d, want 12345", info.PID)
	}
}

func TestIdentityInfo_String(t *testing.T) {
	tests := []struct {
		name string
		info *IdentityInfo
		want []string // 期望包含的子串
	}{
		{
			name: "with username",
			info: &IdentityInfo{
				PeerIdentity: &PeerIdentity{UID: 1000, GID: 1000, PID: 12345},
				Username:     "testuser",
				Groupname:    "testgroup",
			},
			want: []string{"testuser", "testgroup", "pid=12345"},
		},
		{
			name: "without username",
			info: &IdentityInfo{
				PeerIdentity: &PeerIdentity{UID: 1000, GID: 1000, PID: 12345},
			},
			want: []string{"uid=1000", "gid=1000", "pid=12345"},
		},
		{
			name: "partial info",
			info: &IdentityInfo{
				PeerIdentity: &PeerIdentity{UID: 0, GID: 0, PID: 1},
				Username:     "root",
			},
			want: []string{"root", "gid=0", "pid=1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.info.String()
			for _, substr := range tt.want {
				if !strings.Contains(got, substr) {
					t.Errorf("String() = %q, should contain %q", got, substr)
				}
			}
		})
	}
}

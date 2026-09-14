package backend

import "testing"

func TestGitHubCommentTruncatedExcerpts(t *testing.T) {
	for _, tc := range []struct {
		name, side, patch, want string
		start, end              int
		outdated                bool
	}{
		{"new", "RIGHT", "@@ -1,3 +1,5 @@\n function retry() {\n-  return 1000;\n+  const base = 250;\n+  const max = 5000;", "  const max = 5000;", 3, 3, false},
		{"old", "LEFT", "@@ -1,5 +1,6 @@\n {\n-  environment: development,\n-  timeoutMs: 1000,", "  timeoutMs: 1000,", 3, 3, false},
		{"deleted", "LEFT", "@@ -1,3 +0,0 @@\n-Legacy configuration\n-Deprecated timeout: 1000", "Deprecated timeout: 1000", 2, 2, false},
		{"range", "RIGHT", "@@ -1,5 +1,7 @@\n # 部署说明\n \n-使用默认配置启动服务。\n+1. 使用测试配置启动服务。\n+2. 检查超时与重试参数。\n+3. 执行健康检查后再开放流量。", "1. 使用测试配置启动服务。\n2. 检查超时与重试参数。\n3. 执行健康检查后再开放流量。", 3, 5, false},
		{"outdated", "RIGHT", "@@ -1,4 +1,5 @@\n export const limits = {\n-  maxRequests: 100,\n-  timeoutMs: 1000,\n+  maxRequests: 200,\n+  timeoutMs: 2500,", "  timeoutMs: 2500,", 3, 3, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := apiComment{Side: tc.side, DiffHunk: tc.patch, OriginalLine: tc.end, OriginalStartLine: tc.start}
			if !tc.outdated {
				a.Line, a.StartLine = &tc.end, &tc.start
			}
			c := a.comment()
			if c.Code != tc.want || c.Outdated != tc.outdated {
				t.Fatalf("comment excerpt: %+v; want %q", c, tc.want)
			}
		})
	}
}

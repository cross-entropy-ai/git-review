package review

import "testing"

func TestPendingMatchPreservesOtherCommentsAndShiftedAnchors(t *testing.T) {
	draft := Comment{Path: "file.go", Side: "new", Start: 3, End: 5, PendingBody: "same text", Body: "edited text", PendingAfterID: 100, CommitID: "original"}
	remote := Comment{RemoteID: 101, AuthorID: "me", Path: "file.go", Side: "new", StartSide: "new", Start: 10, End: 12, OriginalStart: 3, OriginalEnd: 5, CommitID: "original", Body: "same text"}
	if index, err := MatchPendingComment(draft, []Comment{remote}, "me"); err != nil || index != 0 {
		t.Fatalf("shifted range: %d %v", index, err)
	}
	for _, modify := range []func(*Comment){
		func(c *Comment) { c.RemoteID = 100 },
		func(c *Comment) { c.AuthorID = "other" },
		func(c *Comment) { c.Path = "other.go" },
		func(c *Comment) { c.ReplyTo = 99 },
		func(c *Comment) { c.CommitID = "unrelated" },
		func(c *Comment) { c.Body = "other text" },
	} {
		c := remote
		modify(&c)
		if index, err := MatchPendingComment(draft, []Comment{c}, "me"); err != nil || index >= 0 {
			t.Fatalf("matched another comment: %+v", c)
		}
	}
	draft.PendingBody = ""
	if index, _ := MatchPendingComment(draft, []Comment{remote}, "me"); index >= 0 {
		t.Fatal("deduplicated an intentional new comment")
	}
}

func TestPendingReplyMatchesPromotedRootInSameThread(t *testing.T) {
	draft := Comment{ReplyTo: 41, ThreadID: "thread", PendingBody: "reply", PendingAfterID: 50}
	remote := Comment{RemoteID: 100, ReplyTo: 42, ThreadID: "thread", AuthorID: "me", Body: "reply"}
	if index, err := MatchPendingComment(draft, []Comment{remote}, "me"); err != nil || index != 0 {
		t.Fatalf("promoted thread: %d %v", index, err)
	}
	remote.ThreadID = "other"
	if index, _ := MatchPendingComment(draft, []Comment{remote}, "me"); index >= 0 {
		t.Fatal("matched another discussion")
	}
}

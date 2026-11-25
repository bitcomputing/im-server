package pbobjs

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/dynamicpb"
)

func TestConversationBusinessWireCompatibility(t *testing.T) {
	current := (&Conversation{}).ProtoReflect().Descriptor()
	schema := protodesc.ToFileDescriptorProto(current.ParentFile())
	for _, msg := range schema.MessageType {
		if msg.GetName() == "Conversation" {
			fields := msg.Field
			msg.Field = nil
			for _, f := range fields {
				if f.GetNumber() != 23 {
					msg.Field = append(msg.Field, f)
				}
			}
		}
	}
	oldFile, err := protodesc.NewFile(schema, nil)
	if err != nil {
		t.Fatal(err)
	}
	oldType := oldFile.Messages().ByName("Conversation")
	old := dynamicpb.NewMessage(oldType)
	next := &Conversation{TargetId: "target", Business: &ConversationBusiness{Type: "avatar_pair_group", BusinessId: "pair", AvatarIds: []string{"a", "b"}}}
	wire, err := proto.Marshal(next)
	if err != nil {
		t.Fatal(err)
	}
	if err := proto.Unmarshal(wire, old); err != nil {
		t.Fatal(err)
	}
	if got := old.Get(oldType.Fields().ByName("targetId")).String(); got != "target" {
		t.Fatalf("old reader got target %q", got)
	}
	var read Conversation
	if err := proto.Unmarshal(wire, &read); err != nil || !proto.Equal(next, &read) {
		t.Fatalf("new reader round trip: %v", err)
	}
	old.SetUnknown(nil)
	wire, err = proto.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	if err := proto.Unmarshal(wire, &read); err != nil || read.Business != nil || read.TargetId != "target" {
		t.Fatalf("missing metadata must remain unknown: %v", err)
	}
}

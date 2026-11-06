package entity

import "errors"

type ChatID int64
type ExternalChatID int64
type TopicID int64

type Chat struct {
	ID                  ChatID
	ExternalChatID      ExternalChatID
	SupergroupID        int64
	TopicID             *TopicID
	NextChatOffset      int64
	CommittedChatOffset int64
}

func NewChat(externalChatID ExternalChatID, supergroupID int64) *Chat {
	return &Chat{ExternalChatID: externalChatID, SupergroupID: supergroupID, NextChatOffset: 1}
}

func RehydrateChat(
	id ChatID,
	externalChatID ExternalChatID,
	supergroupID int64,
	topicID *TopicID,
	nextOffset, committedOffset int64,
) (*Chat, error) {
	if id <= 0 || externalChatID == 0 || supergroupID == 0 || nextOffset < 1 || committedOffset < 0 ||
		committedOffset >= nextOffset {
		return nil, errors.New("invalid persisted chat")
	}

	return &Chat{
		ID:                  id,
		ExternalChatID:      externalChatID,
		SupergroupID:        supergroupID,
		TopicID:             topicID,
		NextChatOffset:      nextOffset,
		CommittedChatOffset: committedOffset,
	}, nil
}

func (c *Chat) AllocateOffset() int64 {
	offset := c.NextChatOffset
	c.NextChatOffset++
	return offset
}

func (c *Chat) MapTopic(topicID TopicID) error {
	if topicID <= 0 {
		return errors.New("invalid topic ID")
	}
	c.TopicID = &topicID
	return nil
}

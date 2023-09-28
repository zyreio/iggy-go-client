package binaryserialization

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	. "github.com/iggy-rs/iggy-go-client/contracts"
)

func DeserializeLogInResponse(payload []byte) *LogInResponse {
	userId := binary.LittleEndian.Uint32(payload[0:4])
	return &LogInResponse{
		UserId: userId,
	}
}

func DeserializeOffset(payload []byte) *OffsetResponse {
	partitionId := int(binary.LittleEndian.Uint32(payload[0:4]))
	currentOffset := binary.LittleEndian.Uint64(payload[4:12])
	storedOffset := binary.LittleEndian.Uint64(payload[12:20])

	return &OffsetResponse{
		PartitionId:   partitionId,
		CurrentOffset: currentOffset,
		StoredOffset:  storedOffset,
	}
}

func DeserializeStreams(payload []byte) []StreamResponse {
	streams := make([]StreamResponse, 0)
	position := 0

	for position < len(payload) {
		stream, readBytes := DeserializeToStream(payload, position)
		streams = append(streams, stream)
		position += readBytes
	}

	return streams
}

func DeserializerStream(payload []byte) *StreamResponse {
	stream, position := DeserializeToStream(payload, 0)
	topics := make([]TopicResponse, 0)
	length := len(payload)

	for position < length {
		topic, readBytes, _ := DeserializeToTopic(payload, position)
		topics = append(topics, topic)
		position += readBytes
	}

	return &StreamResponse{
		Id:            stream.Id,
		TopicsCount:   stream.TopicsCount,
		Name:          stream.Name,
		Topics:        topics,
		MessagesCount: stream.MessagesCount,
		SizeBytes:     stream.SizeBytes,
		CreatedAt:     stream.CreatedAt,
	}
}

func DeserializeToStream(payload []byte, position int) (StreamResponse, int) {
	id := int(binary.LittleEndian.Uint32(payload[position : position+4]))
	createdAt := binary.LittleEndian.Uint64(payload[position+4 : position+12])
	topicsCount := int(binary.LittleEndian.Uint32(payload[position+12 : position+16]))
	sizeBytes := binary.LittleEndian.Uint64(payload[position+16 : position+24])
	messagesCount := binary.LittleEndian.Uint64(payload[position+24 : position+32])
	nameLength := int(payload[position+32])

	nameBytes := payload[position+33 : position+33+nameLength]
	name := string(nameBytes)

	readBytes := 4 + 8 + 4 + 8 + 8 + 1 + nameLength

	return StreamResponse{
		Id:            id,
		TopicsCount:   topicsCount,
		Name:          name,
		SizeBytes:     sizeBytes,
		MessagesCount: messagesCount,
		CreatedAt:     createdAt,
	}, readBytes
}

func DeserializeFetchMessagesResponse(payload []byte) (*FetchMessagesResponse, error) {
	const propertiesSize = 45
	length := len(payload)
	partitionId := int(binary.LittleEndian.Uint32(payload[0:4]))
	currentOffset := binary.LittleEndian.Uint64(payload[4:12])
	//messagesCount := int(binary.LittleEndian.Uint32(payload[12:16]))
	position := 16

	response := FetchMessagesResponse{
		PartitionId:   partitionId,
		CurrentOffset: currentOffset,
	}

	if length <= position {
		return &response, nil
	}

	var messages []MessageResponse

	for position < length {
		offset := binary.LittleEndian.Uint64(payload[position : position+8])
		state, err := mapMessageState(payload[position+8])
		timestamp := binary.LittleEndian.Uint64(payload[position+9 : position+17])
		id, err := uuid.FromBytes(payload[position+17 : position+33])
		checksum := binary.LittleEndian.Uint32(payload[position+33 : position+37])
		headersLength := int(binary.LittleEndian.Uint32(payload[position+37 : position+41]))
		headers, err := deserializeHeaders(payload[(position + 41):(position + 41 + headersLength)])
		if err != nil {
			return nil, err
		}
		position += headersLength
		messageLength := binary.LittleEndian.Uint32(payload[position+41 : position+45])

		payloadRangeStart := position + propertiesSize
		payloadRangeEnd := position + propertiesSize + int(messageLength)

		if payloadRangeStart > length || payloadRangeEnd > length {
			break
		}

		payloadSlice := payload[payloadRangeStart:payloadRangeEnd]
		totalSize := propertiesSize + int(messageLength)
		position += totalSize

		messages = append(messages, MessageResponse{
			Id:        id,
			Payload:   payloadSlice,
			Offset:    offset,
			Timestamp: timestamp,
			Checksum:  checksum,
			State:     state,
			Headers:   headers,
		})

		if position+propertiesSize >= length {
			break
		}
	}

	response.Messages = messages
	return &response, nil
}

func mapMessageState(state byte) (MessageState, error) {
	switch state {
	case 1:
		return MessageStateAvailable, nil
	case 10:
		return MessageStateUnavailable, nil
	case 20:
		return MessageStatePoisoned, nil
	case 30:
		return MessageStateMarkedForDeletion, nil
	default:
		return 0, errors.New("Invalid message state")
	}
}

func deserializeHeaders(payload []byte) (map[HeaderKey]HeaderValue, error) {
	headers := make(map[HeaderKey]HeaderValue)
	position := 0

	for position < len(payload) {
		if len(payload) <= position+4 {
			return nil, errors.New("Invalid header key length")
		}

		keyLength := binary.LittleEndian.Uint32(payload[position : position+4])
		position += 4

		if keyLength == 0 || 255 < keyLength {
			return nil, errors.New("Key has incorrect size, must be between 1 and 255")
		}

		if len(payload) < position+int(keyLength) {
			return nil, errors.New("Invalid header key")
		}

		key := string(payload[position : position+int(keyLength)])
		position += int(keyLength)

		headerKind, err := deserializeHeaderKind(payload, position)
		if err != nil {
			return nil, err
		}
		position++

		if len(payload) <= position+4 {
			return nil, errors.New("Invalid header value length")
		}

		valueLength := binary.LittleEndian.Uint32(payload[position : position+4])
		position += 4

		if valueLength == 0 || 255 < valueLength {
			return nil, errors.New("Value has incorrect size, must be between 1 and 255")
		}

		if len(payload) < position+int(valueLength) {
			return nil, errors.New("Invalid header value")
		}

		value := payload[position : position+int(valueLength)]
		position += int(valueLength)

		headers[HeaderKey{Value: key}] = HeaderValue{
			Kind:  headerKind,
			Value: value,
		}
	}

	return headers, nil
}

func deserializeHeaderKind(payload []byte, position int) (HeaderKind, error) {
	if position >= len(payload) {
		return 0, errors.New("Invalid header kind position")
	}

	return HeaderKind(payload[position]), nil
}

func DeserializeTopics(payload []byte) ([]TopicResponse, error) {
	topics := make([]TopicResponse, 0)
	length := len(payload)
	position := 0

	for position < length {
		topic, readBytes, err := DeserializeToTopic(payload, position)
		if err != nil {
			return nil, err
		}
		topics = append(topics, topic)
		position += readBytes
	}

	return topics, nil
}

func DeserializeTopic(payload []byte) (*TopicResponse, error) {
	topic, position, err := DeserializeToTopic(payload, 0)
	if err != nil {
		return &TopicResponse{}, err
	}

	partitions := make([]PartitionContract, 0)
	length := len(payload)

	for position < length {
		partition, readBytes := DeserializePartition(payload, position)
		if err != nil {
			return &TopicResponse{}, err
		}
		partitions = append(partitions, partition)
		position += readBytes
	}

	topic.Partitions = partitions

	return &topic, nil
}

func DeserializeToTopic(payload []byte, position int) (TopicResponse, int, error) {
	topic := TopicResponse{}
	topic.Id = int(binary.LittleEndian.Uint32(payload[position : position+4]))
	topic.CreatedAt = int(binary.LittleEndian.Uint64(payload[position+4 : position+12]))
	topic.PartitionsCount = int(binary.LittleEndian.Uint32(payload[position+12 : position+16]))
	topic.MessageExpiry = int(binary.LittleEndian.Uint32(payload[position+16 : position+20]))
	topic.SizeBytes = binary.LittleEndian.Uint64(payload[position+20 : position+28])
	topic.MessagesCount = binary.LittleEndian.Uint64(payload[position+28 : position+36])

	nameLength := int(payload[position+36])
	nameEnd := position + 37 + nameLength

	if nameEnd > len(payload) {
		return TopicResponse{}, 0, json.Unmarshal([]byte(`{}`), &topic)
	}

	topic.Name = string(bytes.Trim(payload[position+37:nameEnd], "\x00"))

	readBytes := 4 + 4 + 4 + 8 + 8 + 1 + 8 + nameLength
	return topic, readBytes, nil
}

func DeserializePartition(payload []byte, position int) (PartitionContract, int) {
	id := int(binary.LittleEndian.Uint32(payload[position : position+4]))
	createdAt := binary.LittleEndian.Uint64(payload[position+4 : position+12])
	segmentsCount := int(binary.LittleEndian.Uint32(payload[position+12 : position+16]))
	currentOffset := binary.LittleEndian.Uint64(payload[position+16 : position+24])
	sizeBytes := binary.LittleEndian.Uint64(payload[position+24 : position+32])
	messagesCount := binary.LittleEndian.Uint64(payload[position+32 : position+40])
	readBytes := 4 + 4 + 8 + 8 + 8 + 8

	partition := PartitionContract{
		Id:            id,
		CreatedAt:     createdAt,
		SegmentsCount: segmentsCount,
		CurrentOffset: currentOffset,
		SizeBytes:     sizeBytes,
		MessagesCount: messagesCount,
	}

	return partition, readBytes
}

func DeserializeConsumerGroups(payload []byte) []ConsumerGroupResponse {
	var consumerGroups []ConsumerGroupResponse
	length := len(payload)
	position := 0

	for position < length {
		// use slices
		consumerGroup, readBytes := DeserializeToConsumerGroup(payload, position)
		consumerGroups = append(consumerGroups, *consumerGroup)
		position += readBytes
	}

	return consumerGroups
}

func DeserializeConsumerGroup(payload []byte) (*ConsumerGroupResponse, error) {
	consumerGroup, _ := DeserializeToConsumerGroup(payload, 0)
	return consumerGroup, nil
}

func DeserializeToConsumerGroup(payload []byte, position int) (*ConsumerGroupResponse, int) {
	id := int(binary.LittleEndian.Uint32(payload[position : position+4]))
	membersCount := int(binary.LittleEndian.Uint32(payload[position+4 : position+8]))
	partitionsCount := int(binary.LittleEndian.Uint32(payload[position+8 : position+12]))
	readBytes := 12

	consumerGroup := ConsumerGroupResponse{
		Id:              id,
		MembersCount:    membersCount,
		PartitionsCount: partitionsCount,
	}

	return &consumerGroup, readBytes
}
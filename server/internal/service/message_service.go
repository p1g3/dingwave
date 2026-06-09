package service

import (
	"os"
	"path/filepath"
	"strconv"

	"dingtalk/internal/database"

	"gorm.io/gorm"
)

type MessageService struct {
	db *gorm.DB
}

func NewMessageService(db *gorm.DB) *MessageService {
	return &MessageService{db: db}
}

func (s *MessageService) GetConversationMessages(cid string, before, after int64, size int) ([]database.Message, bool, error) {
	var items []database.Message
	query := s.db.Where("cid = ?", cid)

	if before > 0 {
		query = query.Where("created_at < ?", before).Order("created_at DESC")
	} else if after > 0 {
		query = query.Where("created_at > ?", after).Order("created_at ASC")
	} else {
		query = query.Order("created_at DESC")
	}

	query.Limit(size + 1).Find(&items)

	hasMore := false
	if len(items) > size {
		hasMore = true
		items = items[:size]
	}

	if after > 0 {
		for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
			items[i], items[j] = items[j], items[i]
		}
	}

	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}

	return items, hasMore, nil
}

type SearchResultItem struct {
	CID        string                    `json:"cid"`
	Title      string                    `json:"title"`
	Type       database.ConversationType `json:"type"`
	MatchCount int64                     `json:"match_count"`
}

func (s *MessageService) SearchGlobal(query string, page, size int) ([]SearchResultItem, int64, error) {
	offset := (page - 1) * size
	var total int64
	items := []SearchResultItem{}

	s.db.Model(&database.Message{}).
		Where("content_text LIKE ?", "%"+query+"%").
		Distinct("cid").
		Count(&total)

	type AggResult struct {
		CID        string `gorm:"column:cid"`
		MatchCount int64  `gorm:"column:match_count"`
	}
	var aggResults []AggResult
	s.db.Model(&database.Message{}).
		Select("cid, COUNT(*) as match_count").
		Where("content_text LIKE ?", "%"+query+"%").
		Group("cid").
		Order("match_count DESC").
		Limit(size).
		Offset(offset).
		Scan(&aggResults)

	for _, agg := range aggResults {
		var conv database.Conversation
		if err := s.db.Where("cid = ?", agg.CID).First(&conv).Error; err == nil {
			items = append(items, SearchResultItem{
				CID:        agg.CID,
				Title:      conv.Title,
				Type:       conv.Type,
				MatchCount: agg.MatchCount,
			})
		}
	}

	return items, total, nil
}

func (s *MessageService) SearchInConversation(cid, query string, page, size int) ([]database.Message, int64, error) {
	offset := (page - 1) * size
	var total int64
	items := []database.Message{}

	q := s.db.Model(&database.Message{}).Where("cid = ? AND content_text LIKE ?", cid, "%"+query+"%")
	q.Count(&total)
	q.Order("created_at DESC").Limit(size).Offset(offset).Find(&items)

	return items, total, nil
}

func (s *MessageService) PopulateLocalImageURL(messages []database.Message, currentUserID int64) error {
	var imageIDs []int64
	for _, msg := range messages {
		if msg.ContentType == database.MessageContentTypeImage {
			imageIDs = append(imageIDs, msg.ID)
		}
	}

	if len(imageIDs) == 0 {
		return nil
	}

	var mappings []database.ImageMapping
	s.db.Table("image_mappings").Where("mid IN ?", imageIDs).Find(&mappings)

	midToPath := make(map[int64]string)
	for _, m := range mappings {
		midToPath[m.MID] = m.LocalPath
	}

	homeDir, _ := os.UserHomeDir()
	basePath := filepath.Join(homeDir, ".config", "DingTalk", strconv.FormatInt(currentUserID, 10)+"_v2", "resource_cache")

	for i := range messages {
		if messages[i].ContentType != database.MessageContentTypeImage {
			continue
		}
		localPath, ok := midToPath[messages[i].ID]
		if !ok || localPath == "" {
			continue
		}

		relPath, err := filepath.Rel(basePath, localPath)
		if err != nil {
			continue
		}

		messages[i].LocalImageURL = "/cache/" + filepath.ToSlash(relPath)
	}

	return nil
}

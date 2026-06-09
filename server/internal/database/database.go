package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"dingtalk/internal/downloader"
	"dingtalk/internal/logger"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func MigrateToMemory(dbPath string) (*gorm.DB, error) {
	logger.Info("start migrating database")
	db, err := openDB(dbPath)
	if err != nil {
		return nil, err
	}

	memDB, err := createMemoryDB()
	if err != nil {
		return nil, err
	}

	// 迁移 tbuser_profile_v2
	if err := migrateUsers(db, memDB); err != nil {
		return nil, fmt.Errorf("failed to migrate users: %w", err)
	}

	// 迁移 tbconversation
	if err := migrateConversations(db, memDB); err != nil {
		return nil, fmt.Errorf("failed to migrate conversations: %w", err)
	}

	// 合并与迁移钉钉数据库中的多个 tbmessage 表
	if err := migrateMessages(db, memDB); err != nil {
		return nil, fmt.Errorf("failed to migrate messages: %w", err)
	}

	// 迁移图片映射信息
	if err := migrateImageMappings(db, memDB); err != nil {
		return nil, fmt.Errorf("failed to migrate image mappings: %w", err)
	}

	// 更新消息内容文本
	if err := updateContentText(memDB); err != nil {
		return nil, fmt.Errorf("failed to update content text: %w", err)
	}

	// 更新单聊会话标题
	if err := updateSingleChatTitles(memDB); err != nil {
		return nil, fmt.Errorf("failed to update single chat titles: %w", err)
	}

	// 保存当前用户信息
	if err := saveCurrentUser(memDB); err != nil {
		return nil, fmt.Errorf("failed to save current user: %w", err)
	}

	// 更新会话统计信息
	if err := updateConversationStats(memDB); err != nil {
		return nil, fmt.Errorf("failed to update conversation stats: %w", err)
	}

	// 输出统计信息
	if err := logStatistics(memDB); err != nil {
		logger.Warn("failed to log statistics: %v", err)
	}

	return memDB, nil
}

func migrateUsers(srcDB *sql.DB, destDB *gorm.DB) error {
	var users []User
	rows, err := srcDB.Query("SELECT uid,nick,email FROM tbuser_profile_v2")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var user User
		if err := rows.Scan(&user.ID, &user.Nickname, &user.Email); err != nil {
			return err
		}
		users = append(users, user)
	}

	if err := destDB.Create(&users).Error; err != nil {
		return err
	}

	return nil
}

func migrateConversations(srcDB *sql.DB, destDB *gorm.DB) error {
	var conversations []Conversation
	var top int
	rows, err := srcDB.Query("SELECT cid,type,title,top,lastMid,createAt FROM tbconversation")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var conv Conversation
		if err := rows.Scan(&conv.CID, &conv.Type, &conv.Title, &top, &conv.LastMessageID, &conv.CreatedAt); err != nil {
			return err
		}
		if top > 0 {
			conv.IsTop = true
		}
		conversations = append(conversations, conv)
	}

	if err := destDB.Create(&conversations).Error; err != nil {
		return err
	}

	return nil
}

func migrateMessages(srcDB *sql.DB, destDB *gorm.DB) error {
	rows, err := srcDB.Query("SELECT name FROM sqlite_master WHERE type = 'table' AND name LIKE 'tbmsg%';")
	if err != nil {
		return err
	}
	defer rows.Close()

	var tableNames []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return err
		}
		tableNames = append(tableNames, t)
	}

	// 因为 tbmsg 有多个表，gorm 一次性执行的 SQL 长度有上限
	// 为了避免上限，这里使用分片的方式执行
	const batchSize = 1000
	for _, tableName := range tableNames {
		offset := 0
		for {
			query := fmt.Sprintf("SELECT mid, cid, senderId, contentType, content, createdAt, recallStatus FROM %s LIMIT %d OFFSET %d", tableName, batchSize, offset)
			rows, err := srcDB.Query(query)
			if err != nil {
				return err
			}

			var batch []Message
			for rows.Next() {
				var msg Message
				var recallStatus int
				if err := rows.Scan(&msg.ID, &msg.CID, &msg.SenderID, &msg.ContentType, &msg.ContentJson, &msg.CreatedAt, &recallStatus); err != nil {
					return rows.Close()
				}
				if recallStatus > 0 {
					msg.IsRecall = true
				}
				batch = append(batch, msg)
			}
			_ = rows.Close()

			if len(batch) == 0 {
				break
			}

			if err := destDB.Create(&batch).Error; err != nil {
				return err
			}

			offset += batchSize
		}
	}

	return nil
}

func migrateImageMappings(srcDB *sql.DB, destDB *gorm.DB) error {
	var mappings []ImageMapping
	rows, err := srcDB.Query("SELECT url, local_path, mid FROM im_image_info WHERE local_path IS NOT NULL AND local_path != ''")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var mapping ImageMapping
		if err := rows.Scan(&mapping.URL, &mapping.LocalPath, &mapping.MID); err != nil {
			return err
		}
		mappings = append(mappings, mapping)
	}

	if err := destDB.Create(&mappings).Error; err != nil {
		return err
	}

	logger.Info("migrated %d image mappings", len(mappings))
	return nil
}

func updateContentText(db *gorm.DB) error {
	const batchSize = 500
	offset := 0

	for {
		var messages []Message
		if err := db.Limit(batchSize).Offset(offset).Find(&messages).Error; err != nil {
			return err
		}

		if len(messages) == 0 {
			break
		}

		for i := range messages {
			// 给每一种类型的消息增加纯文本字段
			messages[i].ContentText = extractContentText(messages[i].ContentType, messages[i].ContentJson)
		}

		if err := db.Save(&messages).Error; err != nil {
			return err
		}

		offset += batchSize
	}

	return nil
}

func updateSingleChatTitles(db *gorm.DB) error {
	currentUserID, err := getCurrentUserID(db)
	if err != nil {
		return err
	}

	var conversations []Conversation
	if err := db.Where("type = ?", ConversationTypeSingle).Find(&conversations).Error; err != nil {
		return err
	}

	// 单聊字段的 cid 格式为 uid1:uid2，需要替换回真实的用户名称
	for i := range conversations {
		otherUserID, err := GetOtherUserID(conversations[i].CID, currentUserID)
		if err != nil {
			continue
		}

		var user User
		if err := db.First(&user, otherUserID).Error; err == nil {
			conversations[i].Title = user.Nickname
		}
	}

	return db.Save(&conversations).Error
}

func getCurrentUserID(db *gorm.DB) (int64, error) {
	var conversations []Conversation
	if err := db.Where("type = ?", ConversationTypeSingle).Find(&conversations).Error; err != nil {
		return 0, err
	}

	idCount := make(map[int64]int)
	for _, conv := range conversations {
		id1, id2, ok := ParseCID(conv.CID)
		if ok {
			idCount[id1]++
			idCount[id2]++
		}
	}

	return findMostFrequentID(idCount), nil
}

func saveCurrentUser(db *gorm.DB) error {
	currentUserID, err := getCurrentUserID(db)
	if err != nil {
		return err
	}

	var user User
	if err := db.First(&user, currentUserID).Error; err != nil {
		return err
	}

	currentUser := CurrentUser{
		ID:       user.ID,
		Nickname: user.Nickname,
		Email:    user.Email,
	}

	return db.Create(&currentUser).Error
}

func updateConversationStats(db *gorm.DB) error {
	var conversations []Conversation
	if err := db.Find(&conversations).Error; err != nil {
		return err
	}

	// 利用已合并的 tbmsg 表计算以下数据
	// 		单个会话的消息数量
	//		最后消息发送时间
	//		最后消息预览文本
	for i := range conversations {
		var count int64
		if err := db.Model(&Message{}).Where("cid = ?", conversations[i].CID).Count(&count).Error; err != nil {
			return err
		}
		conversations[i].MessageCount = count

		var lastMsg Message
		if err := db.Where("cid = ?", conversations[i].CID).Order("created_at DESC").First(&lastMsg).Error; err == nil {
			conversations[i].LastMessageAt = lastMsg.CreatedAt
			conversations[i].LastMessagePreview = lastMsg.ContentText
		}
	}

	return db.Save(&conversations).Error
}

func openDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

func ValidateDB(dbPath string) error {
	db, err := openDB(dbPath)
	if err != nil {
		return err
	}
	db.Close()
	return nil
}

func createMemoryDB() (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create memory database: %w", err)
	}

	if err := db.AutoMigrate(
		&Conversation{},
		&User{},
		&CurrentUser{},
		&Message{},
		&ImageMapping{},
	); err != nil {
		return nil, fmt.Errorf("failed to migrate database schema: %w", err)
	}

	return db, nil
}

func logStatistics(db *gorm.DB) error {
	var totalMessages int64
	if err := db.Model(&Message{}).Count(&totalMessages).Error; err != nil {
		return err
	}

	var totalConversations int64
	if err := db.Model(&Conversation{}).Count(&totalConversations).Error; err != nil {
		return err
	}

	var topConversations int64
	if err := db.Model(&Conversation{}).Where("is_top = ?", true).Count(&topConversations).Error; err != nil {
		return err
	}

	var singleChats int64
	if err := db.Model(&Conversation{}).Where("type = ?", ConversationTypeSingle).Count(&singleChats).Error; err != nil {
		return err
	}

	var groupChats int64
	if err := db.Model(&Conversation{}).Where("type = ?", ConversationTypeGroup).Count(&groupChats).Error; err != nil {
		return err
	}

	var totalUsers int64
	if err := db.Model(&User{}).Count(&totalUsers).Error; err != nil {
		return err
	}

	logger.Info("total messages: %d", totalMessages)
	logger.Info("total conversations: %d[top: %d, single: %d, group: %d]", totalConversations, topConversations, singleChats, groupChats)
	logger.Info("total users: %d", totalUsers)

	return nil
}

func DownloadImages(db *gorm.DB, token string) error {
	if token == "" {
		return nil
	}

	logger.Info("Image download enabled with token")

	var currentUser CurrentUser
	if err := db.First(&currentUser).Error; err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}

	var messages []Message
	if err := db.Where("content_type = ?", MessageContentTypeImage).Find(&messages).Error; err != nil {
		return fmt.Errorf("failed to query image messages: %w", err)
	}

	if len(messages) == 0 {
		logger.Info("No image messages found")
		return nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	baseDir := filepath.Join(homeDir, ".dingwave", fmt.Sprintf("%d", currentUser.ID), "ddmedia")

	var tasks []downloader.DownloadTask
	for _, msg := range messages {
		info, ok := extractImageInfo(msg.ContentJson)
		if !ok {
			continue
		}

		savePath := filepath.Join(baseDir, info.Filename)
		tasks = append(tasks, downloader.DownloadTask{
			URL:      info.URL,
			SavePath: savePath,
		})
	}

	if len(tasks) == 0 {
		logger.Info("No images to download")
		return nil
	}

	logger.Info("Starting download of %d images", len(tasks))
	downloader.DownloadImagesParallel(tasks, token, 10)
	logger.Info("Image download completed")

	return nil
}

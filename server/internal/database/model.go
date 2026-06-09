package database

type (
	ConversationType   int
	MessageContentType int
)

const (
	ConversationTypeSingle ConversationType = 1
	ConversationTypeGroup  ConversationType = 2
)

const (
	MessageContentTypeText        MessageContentType = 1    // 文本消息
	MessageContentTypeImage       MessageContentType = 2    // 图片消息
	MessageContentTypeDocument    MessageContentType = 4    // 文档消息（PDF等，带直接URL）
	MessageContentTypeShareLink   MessageContentType = 102  // 分享链接消息
	MessageContentTypeLocation    MessageContentType = 202  // 位置消息
	MessageContentTypeLink        MessageContentType = 300  // 链接/分享消息
	MessageContentTypeFileOld     MessageContentType = 500  // 文件消息（旧格式）
	MessageContentTypeFile        MessageContentType = 501  // 文件消息
	MessageContentTypeFolder      MessageContentType = 503  // 文件夹/批量文件
	MessageContentTypeSticker     MessageContentType = 901  // 表情/贴纸消息
	MessageContentTypeCard        MessageContentType = 1101 // 名片消息
	MessageContentTypeVideo       MessageContentType = 1200 // 视频消息
	MessageContentTypeShortVideo  MessageContentType = 1201 // 短视频消息
	MessageContentTypeVideoCall   MessageContentType = 1202 // 视频通话记录
	MessageContentTypeCalendar    MessageContentType = 1500 // 日程/会议消息
	MessageContentTypeVote        MessageContentType = 1600 // 投票消息
	MessageContentTypeRobot       MessageContentType = 2900 // 机器人消息
	MessageContentTypeActionCard  MessageContentType = 2950 // 互动卡片消息
	MessageContentTypeMiniProgram MessageContentType = 3100 // 小程序消息
)

func (t MessageContentType) String() string {
	switch t {
	case MessageContentTypeText:
		return "文本消息"
	case MessageContentTypeImage:
		return "图片消息"
	case MessageContentTypeDocument:
		return "文档消息"
	case MessageContentTypeShareLink:
		return "分享链接"
	case MessageContentTypeLocation:
		return "位置消息"
	case MessageContentTypeLink:
		return "链接消息"
	case MessageContentTypeFileOld:
		return "文件消息(旧)"
	case MessageContentTypeFile:
		return "文件消息"
	case MessageContentTypeFolder:
		return "文件夹消息"
	case MessageContentTypeSticker:
		return "表情贴纸"
	case MessageContentTypeCard:
		return "名片消息"
	case MessageContentTypeVideo:
		return "视频消息"
	case MessageContentTypeShortVideo:
		return "短视频消息"
	case MessageContentTypeVideoCall:
		return "视频通话"
	case MessageContentTypeCalendar:
		return "日程消息"
	case MessageContentTypeVote:
		return "投票消息"
	case MessageContentTypeRobot:
		return "机器人消息"
	case MessageContentTypeActionCard:
		return "互动卡片"
	case MessageContentTypeMiniProgram:
		return "小程序消息"
	default:
		return "未知类型"
	}
}


// tbconversation
type Conversation struct {
	ID                 int64            `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	CID                string           `gorm:"column:cid;index" json:"cid"`                             // 原始数据库存储 CID
	Type               ConversationType `gorm:"column:type;index" json:"type"`                           // 会话类型
	Title              string           `gorm:"column:title" json:"title"`                               // 会话标题
	IsTop              bool             `gorm:"column:is_top;index" json:"is_top"`                       // 是否为置顶会话
	MessageCount       int64            `gorm:"column:message_count" json:"message_count"`               // 会话内消息总数
	LastMessageAt      int64            `gorm:"column:last_message_at;index" json:"last_message_at"`     // 最后消息时间戳
	LastMessageID      int64            `gorm:"column:last_message_id" json:"last_message_id"`           // 最后消息 ID
	LastMessagePreview string           `gorm:"column:last_message_preview" json:"last_message_preview"` // 最后消息预览文本
	CreatedAt          int64            `gorm:"column:created_at" json:"created_at"`                     // 会话创建时间戳
}

// tbuser_profile_v2
type User struct {
	ID       int64  `gorm:"primaryKey;autoIncrement;column:id" json:"id"` // 对应 tbuser_profile_v2 的 uid
	Nickname string `gorm:"column:nickname;index" json:"nickname"`         // 用户昵称
	Email    string `gorm:"column:email;index" json:"email"`               // 用户邮箱
}

// Current user info (singleton)
type CurrentUser struct {
	ID       int64  `gorm:"primaryKey;column:id" json:"id"`
	Nickname string `gorm:"column:nickname" json:"nickname"`
	Email    string `gorm:"column:email" json:"email"`
}

// tbmsg_***
type Message struct {
	ID             int64              `gorm:"primaryKey;autoIncrement;column:id" json:"id"` // 对应 tbmsg_*** 的 mid
	CID            string             `gorm:"column:cid;index" json:"cid"`                  // Conversation 表外键关联 ID
	OriginalCID    string             `gorm:"column:original_cid" json:"original_cid"`      // 原始数据库存储 CID
	SenderID       int64              `gorm:"column:sender_id;index" json:"sender_id"`      // 发送者用户 ID，关联 User 表的 ID
	SenderName     string             `gorm:"-" json:"sender_name"`                         // 发送者昵称（不存储）
	ContentType    MessageContentType `gorm:"column:content_type" json:"content_type"`      // 消息内容类型
	ContentTypeStr string             `gorm:"-" json:"content_type_str"`                    // 消息内容类型字符串（不存储）
	ContentText    string             `gorm:"column:content_text" json:"content_text"`      // 消息文本内容
	ContentJson    string             `gorm:"column:content_json" json:"content_json"`      // 数据库存储原始消息 JSON 内容
	LocalImageURL  string             `gorm:"-" json:"local_image_url,omitempty"`           // 本地高质量图片 URL（运行时计算）

	CreatedAt int64 `gorm:"column:created_at;index" json:"created_at"` // 消息创建时间戳
	IsRecall  bool  `gorm:"column:is_recall" json:"is_recall"`         // 是否为撤回消息
}

// im_image_info (钉钉图片信息映射表)
type ImageMapping struct {
	URL       string `gorm:"column:url" json:"url"`
	LocalPath string `gorm:"column:local_path" json:"local_path"`
	MID       int64  `gorm:"column:mid;index" json:"mid"`
}

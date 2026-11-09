package model

import "time"

// Folder is a node in a user's folder tree. ParentID is empty only for the
// user's root folder (US-01).
type Folder struct {
	ID        string    `json:"id" bson:"_id"`
	Name      string    `json:"name" bson:"name"`
	OwnerID   string    `json:"ownerId" bson:"ownerId"`
	ParentID  string    `json:"parentId,omitempty" bson:"parentId,omitempty"`
	CreatedAt time.Time `json:"createdAt" bson:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt" bson:"updatedAt"`
}

// File holds only metadata. The content lives in the object store and is
// referenced by ObjectKey (US-02, US-12).
type File struct {
	ID          string    `json:"id" bson:"_id"`
	Name        string    `json:"name" bson:"name"`
	ContentType string    `json:"contentType" bson:"contentType"`
	Size        int64     `json:"size" bson:"size"`
	OwnerID     string    `json:"ownerId" bson:"ownerId"`
	ParentID    string    `json:"parentId" bson:"parentId"`
	ObjectKey   string    `json:"-" bson:"objectKey"`
	Hash        string    `json:"-" bson:"hash"`
	CreatedAt   time.Time `json:"createdAt" bson:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt" bson:"updatedAt"`
}

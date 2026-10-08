// Package model mirrors SuperGnosis.Api Model classes.
// JSON tags use camelCase to match ASP.NET Core default serialization,
// and nullable text columns use *string so SQL NULL serializes as null.
package model

// Book mirrors Model/Book.cs (table gnosis.books).
type Book struct {
	ID      int64   `json:"id"`
	Name    *string `json:"name"`
	Perfil  *string `json:"perfil"`
	DriveID *string `json:"driveId"`
}

// BookPage mirrors Model/BookPage.cs (books JOIN pages projection).
type BookPage struct {
	BookID     int64   `json:"bookId"`
	BookName   *string `json:"bookName"`
	PageNumber int64   `json:"pageNumber"`
	PageText   *string `json:"pageText"`
	DriveID    *string `json:"driveId"`
}

// SearchWordAnalytics mirrors Model/SearchWordAnalytics.cs.
// The table has no amount column, so Amount is always 0 (as in .NET,
// where Dapper leaves the default value).
type SearchWordAnalytics struct {
	ID         int64   `json:"id"`
	Perfil     *string `json:"perfil"`
	SearchWord *string `json:"searchWord"`
	Amount     int     `json:"amount"`
}

// User mirrors Model/User.cs (table gnosis.users).
type User struct {
	ID     int64   `json:"id"`
	Name   *string `json:"name"`
	Perfil *string `json:"perfil"`
	Senha  *string `json:"senha"`
}

// LoginResponse mirrors Ok(new { token }) from AuthController.Login.
type LoginResponse struct {
	Token string `json:"token"`
}

// UploadResult is returned by POST /api/book/upload (new endpoint,
// no .NET counterpart).
type UploadResult struct {
	Name   string `json:"name"`
	Perfil string `json:"perfil"`
	Pages  int    `json:"pages"`
}

// ErrorResponse is the JSON error envelope of the new endpoints
// (the ported endpoints keep the original empty bodies).
type ErrorResponse struct {
	Error string `json:"error"`
}

package ports

// Repositories groups the ports bound to one database or transaction.
type Repositories interface {
	Users() UsersRepository
	Projects() ProjectsRepository
	Ratings() RatingsRepository
	Comments() CommentsRepository
	Photos() PhotosRepository
	Contacts() ContactsRepository
	AnonymousTokens() AnonymousTokensRepository
}

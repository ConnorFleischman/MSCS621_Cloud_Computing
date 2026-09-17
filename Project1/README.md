# Project 1 Guidelines
- 3 weeks for project
- Write in Go code, follow AI rules in syllabus
- May be asked to explain snippets of code needed for this project as part of the exam
- Using newsapi.org
    - Create account to use API
    - Collection of news articles to try to access
- Must use Go Channels and Goroutines
- Needs to support multiple invocations
    - Assume there are multiple users that will reach the system at the same time
- Enable searching for a news topic (topic and keyword, find relevant articles)
- User specifies the topic, number of days from today, and maximum number of articles they are interested in
    - Enter 7 days, look from data from all last week. Only want 10 of them, can limit that. Pulling less than limit, just display those articles
- Can use CLI (text-based) or GUI
- Going to use database, can choose Database.
    - When user search, results go to database. If repeated data shows up, go to database first instead of API
    - Examine if database has that data, then go to the API
- Allowed to work in pair
- After midterm week, project presentation for each time, uploaded before class

## Architecture
Go, with MongoDB
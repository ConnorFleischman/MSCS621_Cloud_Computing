1. (Gemini 3.1 Pro) Generate a base GO program that utilizes MongoDB, using Docker.
2. (GPT-6 Astra) Create Go models for News API responses and MongoDB records, make sure it
- Represent title, author, description, URL, source, publication date, topic, and date-range metadata.
- Support JSON decoding and MongoDB serialization.
- Handle missing optional fields safely. (GPT-6 Astra, 9/24)
3. (GPT-6 Astra) Store retrieved articles and coverage metadata in MongoDB, acceptance criteria:
- Associate records with the normalized search topic.
- Store the covered date range and article data.
- Add indexes needed for topic and publication-date queries.
- Prevent duplicate article records.
4. (GPT-6 Astra) Check MongoDB before calling News API. acceptance criteria:
- Repeated searches use cached results when coverage is sufficient.
- News API is not called on a sufficient cache hit.
- Return no more than the requested article limit.
- Call News API on a cache miss.
5. (GPT-6 Astra) Fetch additional data when a request exceeds the cached date range or article count. acceptance criteria:
- Detect an insufficient date range.
- Detect too few cached articles.
- Fetch only when additional data is required.
- Merge new data with cached data without duplicates.
6. (GPT-6 Luna) Present articles in a readable CLI format. acceptance criteria:
- Display title, source, author, publication date, description, and URL when available.
- Indicate when fewer articles are available than requested.
- Indicate whether results came from cache or News API.
- Handle missing fields without broken output. 
7. (GPT-6 Luna) Reconfigure project to have all unit tests put in the same directory as the file they are testing, and ensure the test itself works with this new configuration
8. Test the complete application flow with MongoDB and a mock News API, acceptance criteria:
- Test first search, API retrieval, and persistence.
- Test repeated search from MongoDB.
- Test expanded date and article-limit requests.
- Test concurrent searches.
- Keep production credentials out of tests
9. Look into ways we can reduce the size of the docker image for this project. Come back with 3 solutions, and what changes, if any, are required. 
10. Implement "Multi-stage build with an Alpine runtime" solution, and give new docker command instructions that would fit with this new configuration, if needed. 
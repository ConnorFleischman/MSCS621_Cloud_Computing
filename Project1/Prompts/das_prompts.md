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
7. 
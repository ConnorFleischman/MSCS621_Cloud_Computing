### Basic Commands (Run from the `Project1` folder)

#### 1. Start the Containers

To start all services:

```bash
docker compose up

```

* **Run in the background (detached mode):** If you want to free up your terminal window while the containers run:
```bash
docker compose up -d

```



#### 2. Rebuild and Start (When dependencies or Dockerfiles change)

If you edit `package.json`, `requirements.txt`, or any `Dockerfile`, rebuild the images before launching:

```bash
docker compose up --build

```

#### 3. Stop the Containers

* If running in the foreground: Press `Ctrl + C`.
* If running in detached mode (or in another terminal tab inside `/TimeLore`):
```bash
docker compose down

```



---

### Useful Commands for Terminal Development

* **View Logs:** To watch logs from all containers in real time:
```bash
docker compose logs -f

```


* **View Logs for a Specific Service:** (e.g., just the FastAPI backend):
```bash
docker compose logs -f backend

```


* **Check Status:** See which containers are running and their mapped ports:
```bash
docker compose ps

```
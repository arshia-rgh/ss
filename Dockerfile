FROM golang:1.24.2-bookworm

WORKDIR /app

RUN apt-get update && apt-get install -y wget unzip jq

# install chrome
RUN wget https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb && \
    apt-get install -y ./google-chrome-stable_current_amd64.deb --no-install-recommends && \
    rm google-chrome-stable_current_amd64.deb

# Install chromeDriver, should match the installed Chrome version
RUN CHROME_MAJOR_VERSION=$(google-chrome --product-version | cut -d '.' -f 1) && \
    CHROMEDRIVER_URL=$(wget -qO- "https://googlechromelabs.github.io/chrome-for-testing/latest-versions-per-milestone-with-downloads.json" | jq -r ".milestones[\"$CHROME_MAJOR_VERSION\"].downloads.chromedriver[] | select(.platform==\"linux64\") | .url") && \
    wget -O chromedriver.zip "$CHROMEDRIVER_URL" && \
    unzip chromedriver.zip && \
    mv chromedriver-linux64/chromedriver ./chromedriver && \
    rm -rf chromedriver-linux64 chromedriver.zip && \
    chmod +x ./chromedriver

COPY . .

WORKDIR /app
RUN go mod download
RUN go build -o /app/script1_app .

WORKDIR /app/finall-data
RUN go mod download
RUN go build -o /app/script2_app .

WORKDIR /app
RUN chmod +x /app/run.sh

CMD ["/app/run.sh"]
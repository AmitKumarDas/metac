FROM node:12-buster-slim
COPY server.js /node/
USER 1000
WORKDIR /node
ENTRYPOINT ["node", "/node/server.js"]

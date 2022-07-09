# pull official base image
FROM node:18.4.0-alpine3.15

ENV NPM_CONFIG_LOGLEVEL warn
ARG app_env
ENV NODE_ENV $app_env

RUN mkdir -p /frontend
WORKDIR /frontend

ADD package.json /frontend/package.json
ADD package-lock.json /frontend/package-lock.json
RUN npm install
RUN npm install react-scripts@5.0.1

# add `/app/node_modules/.bin` to $PATH
ENV PATH /frontend/node_modules/.bin:$PATH

# add the app
COPY ./ ./

# if dev settings will use create-react start script for hot code relaoding via docker-compose shared volume
# if production setting will build optimized static files and serve using http-server
CMD if [ ${NODE_ENV} = production ]; \
	then \
	npm install -g serve && \
	npm run build && \
	serve -s build -l 3000; \
	else \
	npm run start; \
	fi

EXPOSE 3000
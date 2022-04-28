# pull official base image
FROM node:13.12.0-alpine

ENV NPM_CONFIG_LOGLEVEL warn
ARG app_env
ENV NODE_ENV $app_env

RUN mkdir -p /frontend
WORKDIR /frontend

ADD package.json /frontend/package.json
RUN npm install --silent
RUN npm install react-scripts@3.4.1 -g --silent

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
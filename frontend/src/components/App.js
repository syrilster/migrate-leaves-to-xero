import React, { Component } from "react";
import Connect from "./connect"
import { Route } from "react-router";
import Upload from "./upload";
import ErrorPage from "./error";
import { BrowserRouter } from "react-router-dom";
import "../App.css";

class App extends Component {
  render() {
    return (
      <div>
        <BrowserRouter>
          <Route exact path="/">
            <Connect />
          </Route>
          <Route exact path="/status">
            <div>
              <h1>Success</h1>
            </div>
          </Route>
          <Route exact path="/upload">
            <Upload />
          </Route>
          <Route exact path="/error">
            <ErrorPage />
          </Route>
        </BrowserRouter>
      </div>
    );
  }
}

export default App;

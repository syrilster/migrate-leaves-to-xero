import React, { Component } from "react";
import "../App.css";

export default class ErrorPage extends Component {
  render() {
    return (
      <div id="error">
        <p className="notFoundDesc">
          <img src="https://user-images.githubusercontent.com/12427222/71851231-f1a1d300-313a-11ea-9d0f-7127cd403487.png" alt="XeroConnectionError"/>
        </p>
      </div>
    );
  }
}

package app

import org.scalajs.dom.document
import slinky.web.ReactDOM
import slinky.web.html._

import app.Base._

object Main {
  def main(args: Array[String]): Unit = {
    val div = document.createElement("div")
    div.id = "root"
    document.body.appendChild(div)

    ReactDOM.render(
      Base.component(),
      document.getElementById("root")
    )
  }
}

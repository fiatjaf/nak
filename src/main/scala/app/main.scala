package app

import org.scalajs.dom.document
import slinky.web.ReactDOM
import slinky.web.html._

object Main {
  def main(args: Array[String]): Unit = {
    val div = document.createElement("div")
    div.id = "root"
    document.body.appendChild(div)
    println("Hello!")

    ReactDOM.render(
      h1("Hello, world?"),
      document.getElementById("root")
    )
  }
}

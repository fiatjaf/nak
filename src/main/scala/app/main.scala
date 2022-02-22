package app

import org.scalajs.dom
import org.scalajs.dom.document

object Main {
  def main(args: Array[String]): Unit = {
    val div = document.createElement("div")
    div.id = "main"
    document.body.appendChild(div)
    println("Hello!")
  }
}
